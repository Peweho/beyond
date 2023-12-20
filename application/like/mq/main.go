package main

import (
	"context"
	"flag"
	"log"

	"myBeyond/application/like/mq/internal/config"
	"myBeyond/application/like/mq/internal/logic"
	"myBeyond/application/like/mq/internal/svc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/service"
)

var configFile = flag.String("f", "etc/like.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)

	svcCtx := svc.NewServiceContext(c)
	ctx := context.Background()
	serviceGroup := service.NewServiceGroup()
	defer serviceGroup.Stop()

	for _, mq := range logic.Consumers(ctx, svcCtx) {
		serviceGroup.Add(mq)
	}

	log.Println("c.KqConsumerConf.Brokers:", c.KqConsumerConf.Brokers)
	serviceGroup.Start()
}