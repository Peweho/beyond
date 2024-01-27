package logic

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"myBeyond/application/article/rpc/internal/code"
	"myBeyond/application/article/rpc/internal/model"
	"myBeyond/application/article/rpc/internal/svc"
	"myBeyond/application/article/rpc/types"
	"myBeyond/application/article/rpc/types/pb"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/mr"
)

const (
	prefixArticles = "biz#articles#%d#%d"
	articlesExpire = 3600 * 24 * 2
)

type ArticlesLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewArticlesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ArticlesLogic {
	return &ArticlesLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// 请求文章列表
func (l *ArticlesLogic) Articles(in *pb.ArticlesRequest) (*pb.ArticlesResponse, error) {
	//1、判断请求参数是否正确并赋上默认值
	if in.SortType != types.SortPublishTime && in.SortType != types.SortLikeCount {
		return nil, code.SortTypeInvalid
	}
	if in.UserId <= 0 {
		return nil, code.UserIdInvalid
	}
	if in.PageSize == 0 {
		in.PageSize = types.DefaultPageSize
	}

	var (
		err       error
		isEnd     bool
		cursor    int64 = in.Cursor
		sortField string
		curPage   []*pb.ArticleItem
		articles  []*model.Article
	)
	//2、查找缓存
	//3.1、判断缓存是否为空并添加结束符
	cacheIds, err := l.cacheArticles(l.ctx, in.UserId, in.Cursor, in.PageSize, in.SortType)

	if err != nil {
		return nil, err
	}
	if len(cacheIds) != 0 {
		//3.2、不为空
		//3.3、转换对象
		articles, err = l.articleByIds(l.ctx, cacheIds)

		if err != nil {
			return nil, err
		}
		curPage = l.ArticleItemByArticle(l.ctx, articles)

		//3.4、构造响应参数
		if cacheIds[len(cacheIds)-1] == -1 {
			isEnd = true
		} else {
			isEnd = false
			cursor += int64(len(cacheIds))
		}

		return &pb.ArticlesResponse{
			Articles:  curPage,
			IsEnd:     isEnd,
			Cursor:    cursor,
			ArticleId: -1,
		}, nil
	}

	//4、若为空查找数据库

	if in.SortType == types.SortPublishTime {
		sortField = "publish_time"
	} else if in.SortType == types.SortLikeCount {
		sortField = "like_count"
	}

	//4.1、查找数据库
	articles, err = l.svcCtx.ArticleModel.ArticlesByUserId(l.ctx, strconv.Itoa(int(in.UserId)), sortField, int(in.Cursor), types.DefaultLimit)
	if err != nil {
		logx.Errorf("ArticlesByUserId userId: %d sortField: %s error: %v", in.UserId, sortField, err)
		return nil, err
	}
	if articles == nil {
		return &pb.ArticlesResponse{}, nil
	}
	if len(articles) < types.DefaultLimit {
		articles = append(articles, &model.Article{Id: -1, LikeNum: 0, PublishTime: time.Date(1970, time.January, 1, 0, 0, 0, 0, time.UTC)})
	}

	//4.2、写缓存
	//threading.GoSafe(
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		err = l.addCacheArticles(l.ctx, articles, in.UserId, in.SortType)
		if err != nil {
			logx.Errorf("addCacheArticles error: %v", err)
		}
		wg.Done()
	}()

	//4.3、转换对象
	articlesLen := len(articles)
	if articlesLen > types.DefaultPageSize {
		articles = articles[:types.DefaultPageSize]
	} else {
		//判断返回的对象是否含有结束位
		if articles[articlesLen-1].Id == -1 {
			isEnd = true
			articles = articles[:articlesLen-1]
		} else {
			isEnd = false
		}
	}

	curPage = l.ArticleItemByArticle(l.ctx, articles)

	cursor += int64(len(curPage))
	fmt.Printf("%d + %d = %d\n", in.Cursor, int64(len(curPage)), cursor)
	fmt.Println(curPage)
	//4.4、构造响应参数
	wg.Wait()
	return &pb.ArticlesResponse{
		Articles:  curPage,
		IsEnd:     isEnd,
		Cursor:    cursor,
		ArticleId: -1,
	}, nil
}

func articlesKey(uid int64, sortType int32) string {
	return fmt.Sprintf(prefixArticles, uid, sortType)
}

func (l *ArticlesLogic) cacheArticles(ctx context.Context, uid, cursor, ps int64, sortType int32) ([]int64, error) {
	//1、构造键名

	key := articlesKey(uid, sortType)
	fmt.Println("1、构造键名", key)
	//2、查询是否存在

	val, err := l.svcCtx.BizRedis.ExistsCtx(ctx, key)
	if err != nil {
		logx.Errorf("ExistsCtx key: %s error: %v", key, err)
		return nil, err
	}
	fmt.Println("2、查询是否存在", val)
	// 3、从新设置有效期

	if val {
		fmt.Println("3、从新设置有效期")
		err := l.svcCtx.BizRedis.ExpireCtx(ctx, key, articlesExpire)
		if err != nil {
			logx.Errorf("ExpireCtx key: %s error: %v", key, err)
		}
	}

	var scoreLimit int64
	if sortType == types.SortPublishTime {
		scoreLimit = time.Now().Unix()
	} else {
		scoreLimit = types.DefaultSortLikeCursor
	}

	// 4、查询缓存
	fmt.Println("4、查询缓存", key, 0, scoreLimit, int(cursor)/int(ps)+1, int(ps))
	pairs, err := l.svcCtx.BizRedis.ZrevrangebyscoreWithScoresAndLimitCtx(ctx, key, 0, scoreLimit, int(cursor)/int(ps), int(ps))
	log.Println(pairs)

	if err != nil {
		logx.Errorf("ZrevrangebyscoreWithScoresAndLimit key: %s error: %v", key, err)
		return nil, err
	}
	// 5、构造结果
	fmt.Println("5、构造结果")
	ids := make([]int64, len(pairs))
	for i, pair := range pairs {
		ids[i], err = strconv.ParseInt(pair.Key, 10, 64)
		if err != nil {
			logx.Errorf("strconv.ParseInt key: %s error: %v", pair.Key, err)
			return nil, err
		}
	}
	fmt.Println("6、返回结果", ids)
	return ids, nil
}

// 根据id转换对象
func (l *ArticlesLogic) articleByIds(ctx context.Context, articleIds []int64) ([]*model.Article, error) {
	articles, err := mr.MapReduce[int64, model.Article, []*model.Article](func(source chan<- int64) {
		for _, id := range articleIds {
			if id == -1 {
				break
			}
			source <- id
		}
	}, func(item int64, writer mr.Writer[model.Article], cancel func(error)) {
		article, err := l.svcCtx.ArticleModel.FindOne(ctx, item)

		log.Println("article1:", article)

		if err != nil {
			cancel(err)
		}
		writer.Write(*article)
	}, func(pipe <-chan model.Article, writer mr.Writer[[]*model.Article], cancel func(error)) {
		articles := make([]*model.Article, 0)

		for article := range pipe {
			log.Println("article2:", article)
			newArticle := article
			articles = append(articles, &newArticle)
		}
		writer.Write(articles)
	})

	if err != nil {
		return nil, err
	}
	return articles, nil
}

func (l *ArticlesLogic) ArticleItemByArticle(ctx context.Context, articles []*model.Article) []*pb.ArticleItem {
	var curPage []*pb.ArticleItem
	for _, article := range articles {
		curPage = append(curPage, &pb.ArticleItem{
			Id:           article.Id,
			Title:        article.Title,
			Content:      article.Content,
			LikeCount:    article.LikeNum,
			CommentCount: article.CommentNum,
			PublishTime:  article.PublishTime.Unix(),
		})
	}
	return curPage
}

// 写缓存
func (l *ArticlesLogic) addCacheArticles(ctx context.Context, articles []*model.Article, userId int64, sortType int32) error {
	key := articlesKey(userId, sortType)
	for _, v := range articles {
		var score int64
		if sortType == types.SortLikeCount {
			score = v.LikeNum
		} else if sortType == types.SortPublishTime {
			score = v.PublishTime.Unix()
		}
		l.svcCtx.BizRedis.ZaddCtx(l.ctx, key, score, strconv.Itoa(int(v.Id)))
	}
	return l.svcCtx.BizRedis.ExpireCtx(l.ctx, key, articlesExpire)
}
