package logic

import (
	"context"
	"strconv"
	"time"

	"myBeyond/application/follow/code"
	"myBeyond/application/follow/rpc/internal/model"
	"myBeyond/application/follow/rpc/internal/svc"
	"myBeyond/application/follow/rpc/internal/types"
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

func (l *FansListLogic) FansList(in *pb.FansListRequest) (*pb.FansListResponse, error) {
	// 1、检验参数
	if in.UserId == 0 {
		return nil, code.UserIdEmpty
	}
	if in.PageSize == 0 {
		in.PageSize = types.DefaultPageSize
	}

	var (
		err            error
		isEnd          bool
		lastId, cursor int64
		follows        []*model.Follow
		curPage        []*pb.FansItem
	)

	// 2、查找缓存
	cacheIds, err := l.cacheIds(l.ctx, in.UserId, in.Cursor, in.PageSize)
	if err != nil {
		return nil, err
	}
	cacheIdsLen := len(cacheIds)
	if cacheIds != nil && cacheIdsLen > 0 {
		// 2.1、判断是否含有标志位
		if cacheIds[cacheIdsLen-1] == -1 {
			cacheIds = cacheIds[:cacheIdsLen]
			isEnd = true
		}
		cacheIdsLen--

		if cacheIdsLen == 0 {
			return &pb.FansListResponse{}, nil
		}
		// 2.2、构造参数
		lastId = cacheIds[cacheIdsLen-1]
		cursor += int64(cacheIdsLen)
		curPage, err = l.itemByIds(l.ctx, in.UserId, cacheIds)
	} else {
		// 3、缓存无，查找数据库
		follows, err = l.svcCtx.FollowModel.FindByFollowedUserId(l.ctx, in.UserId, types.CacheMaxFollowCount, int(in.Cursor))
		if err != nil {
			l.Logger.Errorf("[FollowList] FollowModel.FindByUserId error: %v req: %v", err, in)
			return nil, err
		}
		idsLen := len(follows)
		if idsLen == 0 {
			return &pb.FansListResponse{}, nil
		}

		followIds := make([]int64, 0, idsLen+1)
		for _, val := range follows {
			followIds = append(followIds, val.FollowedUserID)
		}

		// 4、写缓存
		go func() {
			//数量小于上限，加上标志位
			if idsLen < types.CacheMaxFollowCount {
				followIds = append(followIds, -1)
			}
			err = l.addCache(l.ctx, in.UserId, followIds)
			if err != nil {
				logx.Errorf("addCacheFollow error: %v", err)
			}
		}()

		var ids []int64
		if idsLen > int(in.PageSize) {
			ids = followIds[:in.PageSize]
			lastId = ids[in.PageSize-1]
		}
		// 5、构造返回值
		curPage, err = l.itemByIds(l.ctx, in.UserId, ids)
		if err != nil {
			logx.Errorf("databases itemByIds error: %v", err)
		}
		cursor += int64(len(curPage))
	}

	return &pb.FansListResponse{
		Items:  curPage,
		IsEnd:  isEnd,
		Cursor: cursor,
		Id:     lastId,
	}, nil
}

func (l *FansListLogic) cacheIds(ctx context.Context, uid int64, cursor int64, ps int64) ([]int64, error) {
	// 1、构造键名
	fansKey := userFansKey(uid)

	// 2、缓存是否存在
	cacheExist, err := l.svcCtx.BizRedis.ExistsCtx(l.ctx, fansKey)
	if err != nil {
		logx.Errorf("[cacheFollowUserIds] BizRedis.ExistsCtx error: %v", err)
		return nil, err
	}

	// 3、设置有效期
	if cacheExist {
		err = l.svcCtx.BizRedis.ExpireCtx(ctx, fansKey, userFollowExpireTime)
		if err != nil {
			logx.Errorf("[cacheFollowUserIds] BizRedis.ExpireCtx error: %v", err)
			return nil, err
		}
	}

	// 4、查询缓存
	val, err := l.svcCtx.BizRedis.ZrevrangebyscoreWithScoresAndLimitCtx(ctx, fansKey, 0, time.Now().Unix(), int(cursor)/int(ps), int(ps))
	if err != nil {
		logx.Errorf("[cacheFollowUserIds] BizRedis.ZrevrangebyscoreWithScoresAndLimitCtx error: %v", err)
		return nil, err
	}

	// 5、构造结果
	res := make([]int64, 0, len(val))
	for _, v := range val {
		vres, err := strconv.ParseInt(v.Key, 10, 64)
		if err != nil {
			logx.Errorf("[cacheFollowUserIds] strconv.ParseInt error: %v", err)
			continue
		}
		res = append(res, vres)
	}

	return res, nil
}

func (l *FansListLogic) itemByIds(ctx context.Context, uid int64, ids []int64) ([]*pb.FansItem, error) {
	res := make([]*pb.FansItem, 0, len(ids))
	fansCount, err := l.svcCtx.FollowCountModel.FindByUserIds(l.ctx, ids)
	if err != nil {
		l.Logger.Errorf("[FollowList] FollowCountModel.FindByUserIds error: %v", err)
		return nil, err
	}

	for _, val := range fansCount {
		res = append(res, &pb.FansItem{
			UserId:      uid,
			FansUserId:  val.UserID,
			FansCount:   int64(val.FansCount),
			FollowCount: int64(val.FollowCount),
			CreateTime:  val.CreateTime.Unix(),
		})
	}

	return res, nil
}

func (l *FansListLogic) addCache(ctx context.Context, uid int64, ids []int64) error {
	key := userFansKey(uid)
	var score int64
	for _, val := range ids {
		if val == -1 {
			score = 0
		} else {
			score = time.Now().Unix()
		}

		_, err := l.svcCtx.BizRedis.ZaddCtx(l.ctx, key, score, strconv.FormatInt(val, 10))
		if err != nil {
			logx.Errorf("[addCacheFollow] BizRedis.ZaddCtx error: %v", err)
			return err
		}
	}
	return nil
}
