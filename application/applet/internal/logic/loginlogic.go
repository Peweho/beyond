package logic

import (
	"context"
	"fmt"
	"strings"

	"myBeyond/application/applet/internal/code"
	"myBeyond/application/applet/internal/svc"
	"myBeyond/application/applet/internal/types"
	"myBeyond/application/user/rpc/user"
	"myBeyond/pkg/jwt"
	"myBeyond/pkg/xcode"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

type LoginLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewLoginLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LoginLogic {
	return &LoginLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *LoginLogic) Login(req *types.LoginRequest) (*types.LoginResponse, error) {
	//1、获取手机号和验证码
	req.Mobile = strings.TrimSpace(req.Mobile)
	if len(req.Mobile) == 0 {
		return nil, code.LoginMobileEmpty
	}
	req.VerificationCode = strings.TrimSpace(req.VerificationCode)
	if len(req.VerificationCode) == 0 {
		return nil, code.VerificationCodeEmpty
	}
	fmt.Println(req.Mobile + " " + req.VerificationCode)
	//2、判断验证码是否正确
	isLogin, err := checkVerificationCode(l.svcCtx.BizRedis, req.Mobile, req.VerificationCode)
	if err != nil {
		return nil, err
	}
	//2.1、不正确直接返回
	if !isLogin {
		return nil, code.VerificationCodeError
	}
	//3.查找手机号收否存在
	mobile := req.Mobile
	fmt.Println("mobile:" + mobile)
	u, err := l.svcCtx.UserRPC.FindByMobile(l.ctx, &user.FindByMobileRequest{Mobile: mobile})
	if err != nil {
		logx.Errorf("FindByMobile error: %v", err)
		return nil, err
	}
	if u == nil || u.UserId == 0 {
		return nil, xcode.AccessDenied
	}

	//4.生成token
	fmt.Println("token:")
	token, err := jwt.BuildTokens(jwt.TokenOptions{
		AccessSecret: l.svcCtx.Config.Auth.AccessSecret,
		AccessExpire: l.svcCtx.Config.Auth.AccessExpire,
		Fields: map[string]interface{}{
			"userId": u.UserId,
		},
	})
	if err != nil {
		return nil, err
	}
	//5.删除验证码缓存
	delActivationCache(req.Mobile, req.VerificationCode, l.svcCtx.BizRedis)
	return &types.LoginResponse{
		UserId: u.UserId,
		Token: types.Token{
			AccessToken:  token.AccessToken,
			AccessExpire: token.AccessExpire,
		},
	}, nil
}

func checkVerificationCode(rds *redis.Redis, mobile string, verificationCode string) (bool, error) {
	key := fmt.Sprintf(prefixActivation, mobile)
	code, err := rds.Get(key)
	if err != nil {
		return false, err
	}
	if code != verificationCode {
		return false, nil
	} else {
		return true, nil
	}

}
