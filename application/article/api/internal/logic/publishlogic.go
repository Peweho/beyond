package logic

import (
	"context"
	"encoding/json"

	"myBeyond/application/article/api/internal/code"
	"myBeyond/application/article/api/internal/svc"
	"myBeyond/application/article/api/internal/types"
	"myBeyond/application/article/rpc/types/pb"
	"myBeyond/pkg/xcode"

	"github.com/zeromicro/go-zero/core/logx"
)

type PublishLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

const minContentLen = 80

func NewPublishLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PublishLogic {
	return &PublishLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PublishLogic) Publish(req *types.PublishRequest) (resp *types.PublishResponse, err error) {
	if len(req.Title) == 0 {
		return nil, code.ArtitleTitleEmpty
	}
	if len(req.Content) < minContentLen {
		return nil, code.ArticleContentTooFewWords
	}
	if len(req.Cover) == 0 {
		return nil, code.ArticleCoverEmpty
	}

	//获取userId
	userId, err := l.ctx.Value("userId").(json.Number).Int64()
	if err != nil {
		logx.Errorf("l.ctx.Value error: %v", err)
		return nil, xcode.NoLogin
	}
	//远程调用publish
	pret, err := l.svcCtx.ArticleRPC.Publish(l.ctx, &pb.PublishRequest{
		UserId:      userId,
		Title:       req.Title,
		Content:     req.Content,
		Description: req.Description,
		Cover:       req.Cover,
	})
	if err != nil {
		logx.Errorf("l.svcCtx.ArticleRPC.Publish req: %v userId: %d error: %v", req, userId, err)
		return nil, err
	}

	return &types.PublishResponse{ArticleId: pret.ArticleId}, nil
}
