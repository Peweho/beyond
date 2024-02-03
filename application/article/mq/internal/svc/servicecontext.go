package svc

import (
	"myBeyond/application/article/mq/internal/config"
	"myBeyond/application/article/mq/internal/model"
	"myBeyond/application/user/rpc/user"
	"myBeyond/pkg/es"

	//"myBeyond/pkg/es"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
	"github.com/zeromicro/go-zero/zrpc"
)

type ServiceContext struct {
	Config       config.Config
	ArticleModel model.ArticleModel
	BizRedis     *redis.Redis
	UserRPC      user.User
	Es           *es.Es
}

func NewServiceContext(c config.Config) *ServiceContext {
	rds, err := redis.NewRedis(redis.RedisConf{
		Host: c.BizRedis.Host,
		Pass: c.BizRedis.Pass,
		Type: c.BizRedis.Type,
	})
	if err != nil {
		panic(err)
	}

	conn := sqlx.NewMysql(c.Datasource)
	return &ServiceContext{
		Config:       c,
		ArticleModel: model.NewArticleModel(conn),
		BizRedis:     rds,
		UserRPC:      user.NewUser(zrpc.MustNewClient(c.UserRPC)),
		Es: es.MustNewEs(&es.Config{
			Addresses: c.Es.Addresses,
			Username:  c.Es.Username,
			Password:  c.Es.Password,
		}),
	}
}
