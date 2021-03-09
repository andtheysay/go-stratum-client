package main

import (
	"fmt"
	"os"
	"strings"
	"sync"

	stratum "github.com/andtheysay/go-stratum-client"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// set up the zap logger
func init() {
	pe := zap.NewProductionEncoderConfig()
	pe.EncodeLevel = zapcore.CapitalColorLevelEncoder
	encoder := zapcore.NewConsoleEncoder(pe)
	core := zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), zap.DebugLevel)
	zap.ReplaceGlobals(zap.New(core))
}

func main() {
	defer zap.L().Sync()
	server, err := stratum.NewTestServer(8888)
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
			zap.L().Info("Received message", zap.String("message", fmt.Sprintf("%v", clientRequest.Request)))
		}
	}()
	wg.Wait()
}
