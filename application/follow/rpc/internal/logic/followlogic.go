package logic

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"myBeyond/application/follow/code"
	"myBeyond/application/follow/rpc/internal/model"
	"myBeyond/application/follow/rpc/internal/svc"
	"myBeyond/application/follow/rpc/internal/types"
	"myBeyond/application/follow/rpc/pb"

	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
)

type FollowLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewFollowLogic(ctx context.Context, svcCtx *svc.ServiceContext) *FollowLogic {
	return &FollowLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *FollowLogic) Follow(in *pb.FollowRequest) (*pb.FollowResponse, error) {
	// 1、校验参数
	if in.UserId == 0 {
		return nil, code.FollowUserIdEmpty
	}
	if in.FollowedUserId == 0 {
		return nil, code.CannotFollowSelf
	}
	if in.FollowedUserId == in.UserId {
		return nil, code.CannotFollowSelf
	}

	// 2、增加或更新关注表和关注人数表，使用事务实现
	follow, err := l.svcCtx.FollowModel.FindByUserIDAndFollowedUserID(l.ctx, in.UserId, in.FollowedUserId)
	if err != nil {
		l.Logger.Errorf("[Follow] FollowModel.FindByUserIDAndFollowedUserID err: %v req: %v", err, in)
		return nil, err
	}
	if follow != nil && follow.FollowStatus == types.FollowStatusFollow {
		return &pb.FollowResponse{}, nil
	}

	// 2.1、判断关注表中是否有关系，没有关系增加，有关系修改
	err = l.svcCtx.DB.Transaction(func(tx *gorm.DB) error {
		// 增加或更新关注表
		if follow != nil {
			err = model.NewFollowModel(tx).UpdateFields(l.ctx, follow.ID, map[string]any{
				"follow_status": types.FollowStatusFollow,
				"update_time":   time.Now(),
			})
		} else {
			err = model.NewFollowModel(tx).Insert(l.ctx, &model.Follow{
				UserID:         in.UserId,
				FollowedUserID: in.FollowedUserId,
				FollowStatus:   types.FollowStatusFollow,
				CreateTime:     time.Now(),
				UpdateTime:     time.Now(),
			})
		}

		if err != nil {
			return err
		}
		// 更新关注人数表和粉丝人数表
		err = model.NewFollowCountModel(tx).IncrFollowCount(l.ctx, in.UserId)
		if err != nil {
			return err
		}
		return model.NewFollowCountModel(tx).IncrFansCount(l.ctx, in.FollowedUserId)
	})
	if err != nil {
		l.Logger.Errorf("[Follow] Transaction error: %v", err)
		return nil, err
	}
	// 3、缓存，关注列表和粉丝列表
	// 3.1、判断缓存是否存在
	followKey := userFollowKey(in.UserId)
	followExist, err := l.svcCtx.BizRedis.ExistsCtx(l.ctx, followKey)
	if err != nil {
		l.Logger.Errorf("[Follow] Redis Exists error: %v", err)
		return nil, err
	}

	fansKey := userFansKey(in.FollowedUserId)
	fansExist, err := l.svcCtx.BizRedis.ExistsCtx(l.ctx, fansKey)
	if err != nil {
		l.Logger.Errorf("[Fans] Redis Exists error: %v", err)
		return nil, err
	}

	// 3.2存在写缓存
	if followExist {
		_, err = l.svcCtx.BizRedis.ZaddCtx(l.ctx, followKey, time.Now().Unix(), strconv.FormatInt(in.FollowedUserId, 10))
		if err != nil {
			l.Logger.Errorf("[Follow] Redis Zadd error: %v", err)
			return nil, err
		}
		// 3.3、若果超过缓存上限，去掉最旧的
		// ZremrangebyrankCtx，按升序排列，移除下标start~end中的元素
		_, err = l.svcCtx.BizRedis.ZremrangebyrankCtx(l.ctx, followKey, 0, -(types.CacheMaxFollowCount + 1))
		if err != nil {
			l.Logger.Errorf("[Follow] Redis Zremrangebyrank error: %v", err)
			return nil, err
		}
	}

	if fansExist {
		_, err = l.svcCtx.BizRedis.ZaddCtx(l.ctx, fansKey, time.Now().Unix(), strconv.FormatInt(in.FollowedUserId, 10))
		if err != nil {
			l.Logger.Errorf("[Fans] Redis Zadd error: %v", err)
			return nil, err
		}

		_, err = l.svcCtx.BizRedis.ZremrangebyrankCtx(l.ctx, fansKey, 0, -(types.CacheMaxFollowCount + 1))
		if err != nil {
			l.Logger.Errorf("[Follow] Redis Zremrangebyrank error: %v", err)
			return nil, err
		}
	}

	return &pb.FollowResponse{}, nil
}

func userFollowKey(userId int64) string {
	return fmt.Sprintf("biz#user#follow#%d", userId)
}

func userFansKey(userId int64) string {
	return fmt.Sprintf("biz#user#fans#%d", userId)
}
