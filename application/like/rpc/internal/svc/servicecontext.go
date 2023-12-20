package svc

import (
	"myBeyond/application/like/rpc/internal/config"

	"github.com/zeromicro/go-queue/kq"
)

type ServiceContext struct {
	Config config.Config
	KqPusherClient *kq.Pusher
}

func NewServiceContext(c config.Config) *ServiceContext {
	return &ServiceContext{
		Config: c,
	}
}
