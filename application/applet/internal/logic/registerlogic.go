package logic

import (
	"context"
	"errors"
	"strings"

	"myBeyond/application/applet/internal/code"
	"myBeyond/application/applet/internal/svc"
	"myBeyond/application/applet/internal/types"
	"myBeyond/application/user/rpc/user"
	"myBeyond/pkg/encrypt"
	"myBeyond/pkg/jwt"

	"github.com/zeromicro/go-zero/core/logx"
)

type RegisterLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRegisterLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RegisterLogic {
	return &RegisterLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *RegisterLogic) Register(req *types.RegisterRequest) (resp *types.RegisterResponse, err error) {
	// 1、查询手机号是否被注册过
	u, err := l.svcCtx.UserRPC.FindByMobile(l.ctx, &user.FindByMobileRequest{Mobile: req.Mobile})
	if err != nil {
		logx.Errorf("FindByMobile error: %v", err)
		return nil, err
	}

	if u != nil && u.UserId > 0 {
		return nil, code.MobileHasRegistered
	}
	// 2、验证验证码
	req.Mobile = strings.TrimSpace(req.Mobile)
	if len(req.Mobile) == 0 {
		return nil, code.RegisterMobileEmpty
	}

	req.VerificationCode = strings.TrimSpace(req.VerificationCode)
	if len(req.VerificationCode) == 0 {
		return nil, code.VerificationCodeEmpty
	}

	cacheCode, err := getActivationCache(req.Mobile, l.svcCtx.BizRedis)
	if err != nil {
		return nil, err
	} else if cacheCode == "" {
		return nil, errors.New("verification code expired")
	} else if cacheCode != req.VerificationCode {
		return nil, errors.New("verification code failed")
	}

	// 3、密码做加密处理
	req.Password = strings.TrimSpace(req.Password)
	if len(req.Password) == 0 {
		return nil, errors.New("password is empty")
	}
	req.Password = encrypt.EncPassword(req.Password)

	mobile, err := encrypt.EncMobile(req.Mobile)
	if err != nil {
		logx.Errorf("EncMobile mobile: %s error: %v", req.Mobile, err)
		return nil, err
	}

	// 4、注册，返回jwt
	regRet, err := l.svcCtx.UserRPC.Register(l.ctx, &user.RegisterRequest{
		Username: req.Name,
		Mobile:   mobile,
	})
	if err != nil {
		logx.Errorf("Register error: %v", err)
		return nil, err
	}
	token, err := jwt.BuildTokens(jwt.TokenOptions{
		AccessSecret: l.svcCtx.Config.Auth.AccessSecret,
		AccessExpire: l.svcCtx.Config.Auth.AccessExpire,
		Fields: map[string]interface{}{
			"userId": regRet.UserId,
		},
	})
	if err != nil {
		logx.Errorf("BuildTokens error: %v", err)
		return nil, err
	}

	delActivationCache(req.Mobile, req.VerificationCode, l.svcCtx.BizRedis)
	return &types.RegisterResponse{
		UserId: regRet.UserId,
		Token: types.Token{
			AccessToken:  token.AccessToken,
			AccessExpire: token.AccessExpire,
		},
	}, nil
}
