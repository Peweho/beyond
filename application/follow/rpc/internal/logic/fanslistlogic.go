package logic

import (
	"context"

	"myBeyond/application/follow/rpc/internal/svc"
	"myBeyond/application/follow/rpc/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type FansListLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewFansListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *FansListLogic {
	return &FansListLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *FansListLogic) FansList(in *pb.FollowListRequest) (*pb.FollowListResponse, error) {
	// todo: add your logic here and delete this line

	return &pb.FollowListResponse{}, nil
}
