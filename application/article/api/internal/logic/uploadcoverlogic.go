package logic

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"myBeyond/application/article/api/internal/code"
	"myBeyond/application/article/api/internal/svc"
	"myBeyond/application/article/api/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type UploadCoverLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

const maxFileSize = 10 << 20 // 10MB

func NewUploadCoverLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UploadCoverLogic {
	return &UploadCoverLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UploadCoverLogic) UploadCover(req *http.Request) (resp *types.UploadCoverResponse, err error) {
	// fmt.Println("--------------------------------")
	// a, err := l.svcCtx.ArticleRPC.Articles(l.ctx, &pb.ArticlesRequest{
	// 	UserId:   1,
	// 	SortType: 1,
	// 	Cursor:   0,
	// })
	// if err != nil {
	// 	logx.Error(err)
	// }
	// fmt.Println(a)
	// return nil, err

	//解析 multipart/form-data 类型的表单数据
	_ = req.ParseMultipartForm(maxFileSize)
	//获取名为cover的文件对象级信息
	file, handler, err := req.FormFile("cover")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	//获取OSS的bucket
	bucket, err := l.svcCtx.OssClient.Bucket(l.svcCtx.Config.Oss.BucketName)
	if err != nil {
		logx.Errorf("get bucket failed, err: %v", err)
		return nil, code.GetBucketErr
	}

	//生成bucket的键，然后将文件上传到bucket
	objectKey := genFilename(handler.Filename)
	err = bucket.PutObject(objectKey, file)
	if err != nil {
		logx.Errorf("put object failed, err: %v", err)
		return nil, code.PutBucketErr
	}

	return &types.UploadCoverResponse{CoverUrl: genFileURL(objectKey)}, nil

}

func genFilename(filename string) string {
	return fmt.Sprintf("%d_%s", time.Now().UnixMilli(), filename)
}

// 生成访问的URL
func genFileURL(objectKey string) string {
	return fmt.Sprintf("https://pwh-web01.oss-cn-beijing.aliyuncs.com/%s", objectKey)
}
