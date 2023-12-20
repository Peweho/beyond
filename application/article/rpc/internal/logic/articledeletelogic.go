package logic

import (
	"context"

	"myBeyond/application/article/rpc/internal/code"
	"myBeyond/application/article/rpc/internal/svc"
	"myBeyond/application/article/rpc/types"
	"myBeyond/application/article/rpc/types/pb"
	"myBeyond/pkg/xcode"

	"github.com/zeromicro/go-zero/core/logx"
)

type ArticleDeleteLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewArticleDeleteLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ArticleDeleteLogic {
	return &ArticleDeleteLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *ArticleDeleteLogic) ArticleDelete(in *pb.ArticleDeleteRequest) (*pb.ArticleDeleteResponse, error) {
	// 1、检查参数
	if in.UserId <= 0 {
		return nil, code.UserIdInvalid
	}
	if in.ArticleId <= 0 {
		return nil, code.ArticleIdInvalid
	}

	// 2、判断文章是否属于用户
	article, err := l.svcCtx.ArticleModel.FindOne(l.ctx, in.ArticleId)
	if err != nil {
		l.Logger.Errorf("ArticleDelete FindOne req: %v error: %v", in, err)
		return nil, code.ArticleIdNotExist
	}
	if article.AuthorId != in.UserId {
		return nil, xcode.AccessDenied
	}

	// 3、软删除文章
	err = l.svcCtx.ArticleModel.UpdateArticleStatus(l.ctx, in.ArticleId, types.ArticleStatusUserDelete)
	if err != nil {
		l.Logger.Errorf("UpdateArticleStatus req: %v error: %v", in, err)
		return nil, err
	}
	var (
		publishTimeKey = articlesKey(in.UserId, types.SortPublishTime)
		likeNumKey     = articlesKey(in.UserId, types.SortLikeCount)
	)

	// 4、删除文章列表的缓存
	val, _ := l.svcCtx.BizRedis.ExistsCtx(l.ctx, publishTimeKey)
	if val {
		_, err := l.svcCtx.BizRedis.ZremCtx(l.ctx, publishTimeKey, in.ArticleId)
		if err != nil {
			l.Logger.Errorf("ZremCtx req: %v error: %v", in, err)
		}
	}

	val, _ = l.svcCtx.BizRedis.ExistsCtx(l.ctx, likeNumKey)
	if val {
		_, err := l.svcCtx.BizRedis.ZremCtx(l.ctx, likeNumKey, in.ArticleId)
		if err != nil {
			l.Logger.Errorf("ZremCtx req: %v error: %v", in, err)
		}
	}

	return &pb.ArticleDeleteResponse{}, nil
}
