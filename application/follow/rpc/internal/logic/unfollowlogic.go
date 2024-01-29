package logic

import (
	"context"
	"time"

	"myBeyond/application/follow/code"
	"myBeyond/application/follow/rpc/internal/model"
	"myBeyond/application/follow/rpc/internal/svc"
	"myBeyond/application/follow/rpc/internal/types"
	"myBeyond/application/follow/rpc/pb"

	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
)

type UnFollowLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewUnFollowLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UnFollowLogic {
	return &UnFollowLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *UnFollowLogic) UnFollow(in *pb.UnFollowRequest) (*pb.UnFollowResponse, error) {
	// 1、检验参数
	if in.UserId == 0 {
		return nil, code.FollowUserIdEmpty
	}
	if in.FollowedUserId == 0 {
		return nil, code.CannotFollowSelf
	}

	// 2、修改关注状态
	follow, err := l.svcCtx.FollowModel.FindByUserIDAndFollowedUserID(l.ctx, in.UserId, in.FollowedUserId)
	if err != nil {
		l.Logger.Errorf("[UnFollow] FollowModel.FindByUserIDAndFollowedUserID err: %v req: %v", err, in)
		return nil, err
	}

	if follow == nil || follow.FollowStatus == types.FollowStatusUnfollow {
		return &pb.UnFollowResponse{}, nil
	}

	err = l.svcCtx.DB.Transaction(func(tx *gorm.DB) error {
		err = model.NewFollowModel(tx).UpdateFields(l.ctx, follow.ID, map[string]any{
			"follow_status": types.FollowStatusUnfollow,
			"update_time":   time.Now(),
		})
		if err != nil {
			return err
		}

		err = model.NewFollowCountModel(tx).DecrFollowCount(l.ctx, in.UserId)
		if err != nil {
			return err
		}

		return model.NewFollowCountModel(tx).DecrFansCount(l.ctx, in.FollowedUserId)

	})
	if err != nil {
		l.Logger.Errorf("[UnFollow] Transaction error: %v", err)
		return nil, err
	}
	// 3、删除缓存
	_, err = l.svcCtx.BizRedis.ZremCtx(l.ctx, userFollowKey(in.UserId), in.FollowedUserId)
	if err != nil {
		return nil, err
	}

	_, err = l.svcCtx.BizRedis.ZremCtx(l.ctx, userFansKey(in.FollowedUserId), in.UserId)
	if err != nil {
		return nil, err
	}

	return &pb.UnFollowResponse{}, nil
}
