package stratum

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/andtheysay/set"
	jsoniter "github.com/json-iterator/go"
	"go.uber.org/zap"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

var (
	KeepAliveDuration time.Duration = 60 * time.Second
)

type StratumOnWorkHandler func(work *Work)
type StratumContext struct {
	net.Conn
	sync.Mutex
	reader                  *bufio.Reader
	id                      int
	SessionID               string
	KeepAliveDuration       time.Duration
	Work                    *Work
	workListeners           set.Interface
	submitListeners         set.Interface
	responseListeners       set.Interface
	LastSubmittedWork       *Work
	submittedWorkRequestIds set.Interface
	numAcceptedResults      uint64
	numSubmittedResults     uint64
	url                     string
	username                string
	password                string
	connected               bool
	lastReconnectTime       time.Time
	stopChan                chan struct{}
}

func New() *StratumContext {
	sc := &StratumContext{}
	sc.KeepAliveDuration = KeepAliveDuration
	sc.workListeners = set.New(set.ThreadSafe)
	sc.submitListeners = set.New(set.ThreadSafe)
	sc.responseListeners = set.New(set.ThreadSafe)
	sc.submittedWorkRequestIds = set.New(set.ThreadSafe)
	sc.stopChan = make(chan struct{})
	return sc
}

func (sc *StratumContext) Connect(addr string) error {
	defer zap.L().Sync()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}
	zap.L().Debug("stratum-client", zap.String("file", "stratum"), zap.String("func", "Connect"), zap.String("debug", "dial success"))
	sc.url = addr
	sc.Conn = conn
	sc.reader = bufio.NewReader(conn)
	return nil
}

// Call issues a JSONRPC request for the specified serviceMethod.
// It works by calling CallLocked while holding the StratumContext lock
func (sc *StratumContext) Call(serviceMethod string, args interface{}) (*Request, error) {
	sc.Lock()
	defer sc.Unlock()
	return sc.CallLocked(serviceMethod, args)
}

// CallLocked issues a JSONRPC request for the specified serviceMethod.
// The StratumContext lock is expected to be held by the caller
func (sc *StratumContext) CallLocked(serviceMethod string, args interface{}) (*Request, error) {
	defer zap.L().Sync()
	sc.id++

	req := NewRequest(sc.id, serviceMethod, args)
	str, err := req.JsonRPCString()
	if err != nil {
		return nil, err
	}

	if _, err := sc.Write([]byte(str)); err != nil {
		return nil, err
	}
	zap.L().Debug("stratum-client", zap.String("file", "stratum"), zap.String("func", "CallLocked"), zap.String("conn", sc.Conn.LocalAddr().String()), zap.String("str", str))
	return req, nil
}

func (sc *StratumContext) ReadLine() (string, error) {
	line, err := sc.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func (sc *StratumContext) ReadJSON() (map[string]interface{}, error) {
	line, err := sc.reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	var ret map[string]interface{}
	if err = json.Unmarshal([]byte(line), &ret); err != nil {
		return nil, err
	}
	return ret, nil
}

func (sc *StratumContext) ReadResponse() (*Response, error) {
	defer zap.L().Sync()
	line, err := sc.ReadLine()
	if err != nil {
		return nil, err
	}
	line = strings.TrimSpace(line)

	zap.L().Debug("stratum-client", zap.String("file", "stratum"), zap.String("func", "ReadResponse"), zap.String("server sent back", line))
	return ParseResponse([]byte(line))
}

func (sc *StratumContext) Authorize(username, password string) error {
	sc.Lock()
	defer sc.Unlock()
	return sc.authorizeLocked(username, password)
}

func (sc *StratumContext) authorizeLocked(username, password string) error {
	defer zap.L().Sync()
	zap.L().Debug("stratum-client", zap.String("file", "stratum"), zap.String("func", "authorizeLocked"), zap.String("debug", "beginning authorize"))
	args := make(map[string]interface{})
	args["login"] = username
	args["pass"] = password
	args["agent"] = "go-stratum-client"

	_, err := sc.CallLocked("login", args)
	if err != nil {
		return err
	}
	zap.L().Debug("stratum-client", zap.String("file", "stratum"), zap.String("func", "authorizeLocked"), zap.String("debug", "triggered login... awaiting response"))
	response, err := sc.ReadResponse()
	if err != nil {
		return err
	}
	if response.Error != nil {
		return response.Error
	}

	sc.connected = true
	sc.username = username
	sc.password = password

	sid, ok := response.Result["id"]
	if !ok {
		return fmt.Errorf("Response did not have a sessionID: %v", response.String())
	}
	sc.SessionID = sid.(string)
	work, err := ParseWork(response.Result["job"].(map[string]interface{}))
	if err != nil {
		return err
	}
	zap.L().Debug("stratum-client", zap.String("file", "stratum"), zap.String("func", "authorizeLocked"), zap.String("debug", "authorization successful"))
	sc.NotifyNewWork(work)

	// Handle messages
	go sc.RunHandleMessages()
	// Keep-alive
	go sc.RunKeepAlive()

	zap.L().Debug("stratum-client", zap.String("file", "stratum"), zap.String("func", "authorizeLocked"), zap.String("debug", "returning from authorizeLocked"))
	return nil
}

func (sc *StratumContext) RunKeepAlive() {
	defer zap.L().Sync()
	sendKeepAlive := func() {
		args := make(map[string]interface{})
		args["id"] = sc.SessionID
		if _, err := sc.Call("keepalived", args); err != nil {
			zap.L().Error("stratum-client", zap.String("file", "stratum"), zap.String("func", "RunKeepAlive"), zap.Error(err))
		} else {
			zap.L().Debug("stratum-client", zap.String("file", "stratum"), zap.String("func", "RunKeepAlive"), zap.String("debug", "posted keepalive"))
		}
	}

	for {
		select {
		case <-sc.stopChan:
			return
		case <-time.After(sc.KeepAliveDuration):
			go sendKeepAlive()
		}
	}
}

func (sc *StratumContext) RunHandleMessages() {
	defer zap.L().Sync()
	// This loop only ends on error
	defer func() {
		sc.Reconnect()
	}()

	for {
		line, err := sc.ReadLine()
		if err != nil {
			zap.L().Debug("stratum-client", zap.String("file", "stratum"), zap.String("func", "RunHandleMessages"), zap.Error(err))
			break
		}
		zap.L().Debug("stratum-client", zap.String("file", "stratum"), zap.String("func", "RunHandleMessages"), zap.String("received line from server", line))
		var msg map[string]interface{}
		if err = json.Unmarshal([]byte(line), &msg); err != nil {
			zap.L().Error("stratum-client", zap.String("file", "stratum"), zap.String("func", "RunHandleMessages"), zap.String("error", "failed to unmarshal line into JSON"), zap.String("line", line), zap.Error(err))
			break
		}

		id := msg["id"]
		switch id.(type) {
		case uint64, float64:
			// This is a response
			response, err := ParseResponse([]byte(line))
			if err != nil {
				zap.L().Debug("stratum-client", zap.String("file", "stratum"), zap.String("func", "RunHandleMessages"), zap.String("error", "failed to parse response from server"), zap.Error(err))
				continue
			}
			isError := false
			if response.Result == nil {
				// This is an error
				isError = true
			}
			id := uint64(response.MessageID.(float64))
			if sc.submittedWorkRequestIds.Has(id) {
				if !isError {
					// This is a response from the server signalling that our work has been accepted
					sc.submittedWorkRequestIds.Remove(id)
					sc.numAcceptedResults++
					sc.numSubmittedResults++
					zap.L().Info("stratum-client", zap.String("file", "stratum"), zap.String("func", "RunHandleMessages"), zap.Uint64("numAcceptedResults", sc.numAcceptedResults), zap.Uint64("numSubmittedResults", sc.numSubmittedResults))
				} else {
					sc.submittedWorkRequestIds.Remove(id)
					sc.numSubmittedResults++
					zap.L().Error("stratum-client", zap.String("file", "stratum"), zap.String("func", "RunHandleMessages"), zap.Uint64("numRejectedResults", sc.numSubmittedResults-sc.numAcceptedResults),
						zap.Uint64("numSubmittedResults", sc.numSubmittedResults), zap.Error(response.Error))
				}
			} else {
				statusIntf, ok := response.Result["status"]
				if !ok {
					zap.L().Warn("stratum-client", zap.String("file", "stratum"), zap.String("func", "RunHandleMessages"), zap.String("server sent back unknown message", response.String()))
				} else {
					status := statusIntf.(string)
					switch status {
					case "KEEPALIVED":
						// Nothing to do
					case "OK":
						zap.L().Error("stratum-client", zap.String("file", "stratum"), zap.String("func", "RunHandleMessages"), zap.String("error", "failed to properly mark submitted work as accepted"),
							zap.String("work id", fmt.Sprintf("%v", response.MessageID)), zap.String("message", response.String()), zap.String("works", fmt.Sprintf("%v", sc.submittedWorkRequestIds.List())))
					}
				}
			}
			sc.NotifyResponse(response)
		default:
			// this is a notification
			// TODO don't use fmt.sprintf to format interfaces?
			zap.L().Debug("stratum-client", zap.String("file", "stratum"), zap.String("func", "RunHandleMessages"), zap.String("received message from stratum server", fmt.Sprintf("%v", msg)))
			switch msg["method"].(string) {
			case "job":
				if work, err := ParseWork(msg["params"].(map[string]interface{})); err != nil {
					zap.L().Error("stratum-client", zap.String("file", "stratum"), zap.String("func", "RunHandleMessages"), zap.String("error", "failed to parse job"), zap.Error(err))
					continue
				} else {
					sc.NotifyNewWork(work)
				}
			default:
				zap.L().Error("stratum-client", zap.String("file", "stratum"), zap.String("func", "RunHandleMessages"), zap.String("unknown method", fmt.Sprintf("%v", msg["method"])))
			}
		}
	}
}

func (sc *StratumContext) Reconnect() {
	defer zap.L().Sync()
	sc.Lock()
	defer sc.Unlock()
	sc.stopChan <- struct{}{}
	if sc.Conn != nil {
		sc.Close()
		sc.Conn = nil
	}
	reconnectTimeout := 1 * time.Second
	for {
		zap.L().Info("stratum-client", zap.String("file", "stratum"), zap.String("func", "Reconnect"), zap.String("info", "reconnecting"))
		now := time.Now()
		if now.Sub(sc.lastReconnectTime) < reconnectTimeout {
			time.Sleep(reconnectTimeout) //XXX: Should we sleeping the remaining time?
		}
		if err := sc.Connect(sc.url); err != nil {
			// TODO: We should probably try n-times before crashing
			zap.L().Info("stratum-client", zap.String("file", "stratum"), zap.String("func", "Reconnect"), zap.String("error", "failed to reconnect"), zap.String("url", sc.url), zap.Error(err))
			reconnectTimeout = 5 * time.Second
		} else {
			break
		}
	}
	zap.L().Debug("stratum-client", zap.String("file", "stratum"), zap.String("func", "Reconnect"), zap.String("debug", "connected. authorizing..."))
	sc.authorizeLocked(sc.username, sc.password)
}

func (sc *StratumContext) SubmitWork(work *Work, hash string) error {
	defer zap.L().Sync()
	if work == sc.LastSubmittedWork {
		// TODO should this return nil? this was done by the og dev
		zap.L().Warn("stratum-client", zap.String("file", "stratum"), zap.String("func", "SubmitWork"), zap.String("warn", "prevented submission of stale work"))
		// return nil
	}
	args := make(map[string]interface{})
	nonceStr, err := BinToHex(work.Data[39:43])
	if err != nil {
		return err
	}
	args["id"] = sc.SessionID
	args["job_id"] = work.JobID
	args["nonce"] = nonceStr
	args["result"] = hash
	if req, err := sc.Call("submit", args); err != nil {
		return err
	} else {
		sc.submittedWorkRequestIds.Add(uint64(req.MessageID.(int)))
		// Successfully submitted result
		zap.L().Debug("stratum-client", zap.String("file", "stratum"), zap.String("func", "SubmitWork"), zap.String("info", "successfully submitted work result"), zap.String("id", work.JobID), zap.String("hash", hash))
		args["work"] = work
		sc.NotifySubmit(args)
		sc.LastSubmittedWork = work
	}
	return nil
}

func (sc *StratumContext) RegisterSubmitListener(sChan chan interface{}) {
	defer zap.L().Sync()
	zap.L().Debug("stratum-client", zap.String("file", "stratum"), zap.String("func", "RegisterSubmitListener"), zap.String("debug", "registered stratum.submitListener"))
	sc.submitListeners.Add(sChan)
}

func (sc *StratumContext) RegisterWorkListener(workChan chan *Work) {
	defer zap.L().Sync()
	zap.L().Debug("stratum-client", zap.String("file", "stratum"), zap.String("func", "RegisterSubmitListener"), zap.String("debug", "registered stratum.workListener"))
	sc.workListeners.Add(workChan)
}

func (sc *StratumContext) RegisterResponseListener(rChan chan *Response) {
	defer zap.L().Sync()
	zap.L().Debug("stratum-client", zap.String("file", "stratum"), zap.String("func", "RegisterResponseListener"), zap.String("debug", "registered stratum.responseListener"))
	sc.responseListeners.Add(rChan)
}

func (sc *StratumContext) GetJob() error {
	args := make(map[string]interface{})
	args["id"] = sc.SessionID
	_, err := sc.Call("getjob", args)
	return err
}

func ParseResponse(b []byte) (*Response, error) {
	var response Response
	if err := json.Unmarshal(b, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (sc *StratumContext) NotifyNewWork(work *Work) {
	defer zap.L().Sync()
	if (sc.Work != nil && strings.Compare(work.JobID, sc.Work.JobID) == 0) || sc.submittedWorkRequestIds.Has(work.JobID) {
		zap.L().Warn("stratum-client", zap.String("file", "stratum"), zap.String("func", "NotifyNewWork"), zap.String("debug", "duplicate job request"), zap.String("reconnecting to ", sc.url))
		// Just disconnect
		sc.connected = false
		sc.Close()
		return
	}
	zap.L().Info("stratum-client", zap.String("file", "stratum"), zap.String("func", "NotifyNewWork"), zap.String("new job", sc.url), zap.Float64("difficulty", work.Difficulty))
	sc.Work = work
	for _, obj := range sc.workListeners.List() {
		ch := obj.(chan *Work)
		ch <- work
	}
}

func (sc *StratumContext) NotifySubmit(data interface{}) {
	for _, obj := range sc.submitListeners.List() {
		ch := obj.(chan interface{})
		ch <- data
	}
}

func (sc *StratumContext) NotifyResponse(response *Response) {
	for _, obj := range sc.responseListeners.List() {
		ch := obj.(chan *Response)
		ch <- response
	}
}

func (sc *StratumContext) Lock() {
	sc.Mutex.Lock()
}

func (sc *StratumContext) lockDebug() {
	defer zap.L().Sync()
	sc.Mutex.Lock()
	zap.L().Debug("stratum-client", zap.String("file", "stratum"), zap.String("func", "lockDebug"), zap.String("mutex lock acquired by", MyCaller()))
}

func (sc *StratumContext) Unlock() {
	sc.Mutex.Unlock()
}

func (sc *StratumContext) unlockDebug() {
	defer zap.L().Sync()
	sc.Mutex.Unlock()
	zap.L().Debug("stratum-client", zap.String("file", "stratum"), zap.String("func", "unlockDebug"), zap.String("mutex unlocked by", MyCaller()))
}
