package stratum

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"unsafe"

	"go.uber.org/zap"
)

type Work struct {
	Data       WorkData
	Target     uint64 `json:"target"`
	JobID      string `json:"job_id"`
	NoncePtr   *uint32
	Difficulty float64 `json:"difficulty"`
	XNonce2    string
	Size       int
}

func NewWork() *Work {
	work := &Work{}
	work.Data = make([]byte, 96)
	work.Target = 0
	work.NoncePtr = (*uint32)(unsafe.Pointer(&work.Data[39]))
	return work
}

type WorkData []byte

func ParseWorkFromResponse(r *Response) (*Work, error) {
	result := r.Result
	if job, ok := result["job"]; !ok {
		return nil, fmt.Errorf("No job found")
	} else {
		return ParseWork(job.(map[string]interface{}))
	}
}

func ParseWork(args map[string]interface{}) (*Work, error) {
	defer zap.L().Sync()
	jobId := args["job_id"].(string)
	hexBlob := args["blob"].(string)
	blobLen := len(hexBlob)
	zap.L().Debug("stratum-client", zap.String("file", "work"), zap.String("func", "ParseWork"), zap.String("jobId", jobId), zap.String("hexBlob", hexBlob), zap.Int("blobLen", blobLen))

	if blobLen%2 != 0 || ((blobLen/2) < 40 && blobLen != 0) || (blobLen/2) > 128 {
		return nil, fmt.Errorf("JSON invalid blob length")
	}

	if blobLen == 0 {
		return nil, fmt.Errorf("Blob length was 0?")
	}

	// TODO: Should there be a lock here?
	blob, err := HexToBin(hexBlob, blobLen)
	if err != nil {
		return nil, err
	}

	zap.L().Debug("stratum-client", zap.String("file", "work"), zap.String("func", "ParseWork"), zap.ByteString("blob bytes", blob))

	targetStr := args["target"].(string)
	zap.L().Debug("stratum-client", zap.String("file", "work"), zap.String("func", "ParseWork"), zap.String("targetStr", targetStr))
	b, err := HexToBin(targetStr, 8)
	target := uint64(binary.LittleEndian.Uint32(b))
	target64 := math.MaxUint64 / (uint64(0xFFFFFFFF) / target)
	target = target64
	difficulty := float64(0xFFFFFFFFFFFFFFFF) / float64(target64)
	zap.L().Debug("stratum-client", zap.String("file", "work"), zap.String("func", "ParseWork"), zap.Uint64("target set", target), zap.Float64("difficulty set", difficulty))

	work := NewWork()

	copy(work.Data, blob)
	// XXX: Do we need to do this?
	for i := len(blob); i < len(work.Data); i++ {
		work.Data[i] = '\x00'
	}

	work.Size = blobLen / 2
	work.JobID = jobId
	work.Target = target
	work.Difficulty = difficulty
	return work, nil
}

func WorkCopy(dest *Work, src *Work) {
	copy(dest.Data, src.Data)
	dest.Size = src.Size
	dest.Difficulty = src.Difficulty
	dest.Target = src.Target
	if strings.Compare(src.JobID, "") != 0 {
		dest.JobID = src.JobID
	}
	if strings.Compare(src.XNonce2, "") != 0 {
		dest.XNonce2 = src.XNonce2
	}
}
