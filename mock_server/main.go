package main

import (
	"fmt"
	"os"
	"strings"
	"sync"

	stratum "github.com/andtheysay/go-stratum-client"
	flag "github.com/spf13/pflag"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var port *int

func init() {
	// set up the zap logger
	pe := zap.NewProductionEncoderConfig()
	pe.EncodeLevel = zapcore.CapitalColorLevelEncoder
	encoder := zapcore.NewConsoleEncoder(pe)
	core := zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), zap.DebugLevel)
	zap.ReplaceGlobals(zap.New(core))
	// set the port
	port = flag.IntP("port", "p", 8888, "port to run mock stratum server")
	flag.Parse()
}

func main() {
	defer zap.L().Sync()
	server, err := stratum.NewTestServer(*port)
	if err != nil {
		zap.L().Fatal("unable to connect", zap.Error(err))
	}
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer zap.L().Sync()
		for clientRequest := range server.RequestChan {
			if strings.Compare(clientRequest.Request.RemoteMethod, "login") == 0 || strings.Compare(clientRequest.Request.RemoteMethod, "submit") == 0 {
				if _, err := clientRequest.Conn.Write([]byte(stratum.TEST_JOB_STR_5)); err != nil {
					zap.L().Error("failed to send client test job", zap.Error(err))
				}
			}
			zap.L().Info("received message", zap.String("message", fmt.Sprintf("%v", clientRequest.Request)))
		}
	}()
	wg.Wait()
}
