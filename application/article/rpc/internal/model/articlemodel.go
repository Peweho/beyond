package model

import (
	"context"
	"fmt"

	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ ArticleModel = (*customArticleModel)(nil)

type (
	// ArticleModel is an interface to be customized, add more methods here,
	// and implement the added methods in customArticleModel.
	ArticleModel interface {
		articleModel
		ArticlesByUserId(ctx context.Context, userId, sortField string, offset, limit int) ([]*Article, error)
		UpdateArticleStatus(ctx context.Context, id int64, status int) error
	}

	customArticleModel struct {
		*defaultArticleModel
	}
)

// NewArticleModel returns a model for the database table.
func NewArticleModel(conn sqlx.SqlConn, c cache.CacheConf, opts ...cache.Option) ArticleModel {
	return &customArticleModel{
		defaultArticleModel: newArticleModel(conn, c, opts...),
	}
}

func (m *customArticleModel) ArticlesByUserId(ctx context.Context, userId, sortField string, offset, limit int) ([]*Article, error) {
	var articles []*Article
	sql := fmt.Sprintf("select " + articleRows + " from " + m.table + " where author_id = ? order by ? desc limit ?,?")
	err := m.QueryRowsNoCacheCtx(ctx, &articles, sql, userId, sortField, offset, limit)
	if err != nil {
		return nil, err
	}
	return articles, nil
}

func (m *customArticleModel) UpdateArticleStatus(ctx context.Context, id int64, status int) error {
	return nil
}
