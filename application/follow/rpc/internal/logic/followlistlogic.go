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

const userFollowExpireTime = 3600 * 24 * 2

type FollowListLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewFollowListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *FollowListLogic {
	return &FollowListLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *FollowListLogic) FollowList(in *pb.FollowListRequest) (*pb.FollowListResponse, error) {
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
		curPage        []*pb.FollowItem
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
			return &pb.FollowListResponse{}, nil
		}
		// 2.2、构造参数
		lastId = cacheIds[cacheIdsLen-1]
		cursor += int64(cacheIdsLen)
		curPage, err = l.itemByIds(l.ctx, in.UserId, cacheIds)
	} else {
		// 3、缓存无，查找数据库
		follows, err = l.svcCtx.FollowModel.FindByUserId(l.ctx, in.UserId, types.CacheMaxFollowCount, int(in.Cursor))
		if err != nil {
			l.Logger.Errorf("[FollowList] FollowModel.FindByUserId error: %v req: %v", err, in)
			return nil, err
		}
		idsLen := len(follows)
		if idsLen == 0 {
			return &pb.FollowListResponse{}, nil
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

	return &pb.FollowListResponse{
		Items:  curPage,
		IsEnd:  isEnd,
		Cursor: cursor,
		Id:     lastId,
	}, nil
}

func (l *FollowListLogic) cacheIds(ctx context.Context, uid int64, cursor int64, ps int64) ([]int64, error) {
	// 1、构造键名
	followKey := userFollowKey(uid)

	// 2、缓存是否存在
	cacheExist, err := l.svcCtx.BizRedis.ExistsCtx(l.ctx, followKey)
	if err != nil {
		logx.Errorf("[cacheFollowUserIds] BizRedis.ExistsCtx error: %v", err)
		return nil, err
	}

	// 3、设置有效期
	if cacheExist {
		err = l.svcCtx.BizRedis.ExpireCtx(ctx, followKey, userFollowExpireTime)
		if err != nil {
			logx.Errorf("[cacheFollowUserIds] BizRedis.ExpireCtx error: %v", err)
			return nil, err
		}
	}

	// 4、查询缓存
	val, err := l.svcCtx.BizRedis.ZrevrangebyscoreWithScoresAndLimitCtx(ctx, followKey, 0, time.Now().Unix(), int(cursor)/int(ps), int(ps))
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

func (l *FollowListLogic) itemByIds(ctx context.Context, uid int64, ids []int64) ([]*pb.FollowItem, error) {
	res := make([]*pb.FollowItem, 0, len(ids))
	followUser, err := l.svcCtx.FollowModel.FindByFollowedUserIds(l.ctx, uid, ids)
	if err != nil {
		l.Logger.Errorf("[FollowList] FollowModel.FindByFollowedUserIds error: %v", err)
		return nil, err
	}

	followCount, err := l.svcCtx.FollowCountModel.FindByUserIds(l.ctx, ids)
	if err != nil {
		l.Logger.Errorf("[FollowList] FollowCountModel.FindByUserIds error: %v", err)
		return nil, err
	}

	var userIdCount map[int64]int64
	for _, val := range followCount {
		userIdCount[val.UserID] = int64(val.FansCount)
	}

	for _, val := range followUser {
		res = append(res, &pb.FollowItem{
			Id:             val.ID,
			FollowedUserId: val.FollowedUserID,
		})
	}

	for _, val := range res {
		val.FansCount = userIdCount[val.FollowedUserId]
	}

	return res, nil
}

func (l *FollowListLogic) addCache(ctx context.Context, uid int64, ids []int64) error {
	key := userFollowKey(uid)
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
