package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/juggleim/jugglechat-server/commons/configures"
	"github.com/juggleim/jugglechat-server/commons/dbcommons"
	"github.com/juggleim/jugglechat-server/log"
	"github.com/juggleim/jugglechat-server/routers"
	"github.com/juggleim/jugglechat-server/services/deviceauth"
)

func main() {
	//init configure
	if err := configures.InitConfigures(); err != nil {
		fmt.Println("Init Configures failed", err)
		return
	}
	//init log
	log.InitLogs()
	//init mysql
	if err := dbcommons.InitMysql(); err != nil {
		log.Error("Init Mysql failed.", err)
		return
	}
	//upgrade db
	if err := dbcommons.Upgrade(); err != nil {
		log.Error("Database migration failed", err)
		return
	}
	workerCtx, stopWorker := context.WithCancel(context.Background())
	defer stopWorker()
	go func() {
		store := deviceauth.Store{DB: dbcommons.GetDb(), IM: deviceauth.ConfiguredIM()}
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			if err := store.Recover(workerCtx, time.Now().UnixMilli()); err != nil && workerCtx.Err() == nil {
				log.Error("Device revocation recovery database error", err)
			}
			select {
			case <-workerCtx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	httpServer := gin.Default()
	routers.Route(httpServer, "jim")
	routers.LoadWebIM(httpServer)
	go httpServer.Run(fmt.Sprintf(":%d", configures.Config.Port))
	if configures.Config.CallbackPort > 0 {
		callbackServer := gin.Default()
		routers.CallbackRoute(callbackServer, "callback")
		// 回调只服务于同机引擎：默认只绑回环，公网与内网都碰不到。
		// 引擎与业务层分机部署时，用 callbackBindAddr 显式放开，并自行加防火墙规则。
		bindAddr := configures.Config.CallbackBindAddr
		if bindAddr == "" {
			bindAddr = "127.0.0.1"
		}
		go callbackServer.Run(fmt.Sprintf("%s:%d", bindAddr, configures.Config.CallbackPort))
	}

	closeChan := make(chan struct{})
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		<-sigChan
		signal.Stop(sigChan)
		close(closeChan)
	}()

	<-closeChan
}
