// Copyright © 2023 OpenIM open source community. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package admin

import (
	"context"
	"github.com/openimsdk/chat/pkg/common/tokenverify"
	"openim38.local/identity"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/openimsdk/chat/pkg/eerrs"
	adminpb "github.com/openimsdk/chat/pkg/protocol/admin"
	"github.com/openimsdk/tools/errs"
	"github.com/openimsdk/tools/log"
	"google.golang.org/grpc/metadata"
)

func (o *adminServer) CreateToken(ctx context.Context, req *adminpb.CreateTokenReq) (*adminpb.CreateTokenResp, error) {
	issuer := o.Token
	if identity.Required() && req.UserType == 1 {
		issuer = &tokenverify.Token{Secret: o.Token.Secret, Expires: 4 * time.Hour}
	}
	token, expire, err := issuer.CreateToken(req.UserID, req.UserType)
	if err != nil {
		return nil, err
	}
	err = o.Database.CacheToken(ctx, req.UserID, token, expire)
	if err != nil {
		return nil, err
	}
	return &adminpb.CreateTokenResp{
		Token: token,
	}, nil
}

func (o *adminServer) ParseToken(ctx context.Context, req *adminpb.ParseTokenReq) (*adminpb.ParseTokenResp, error) {
	userID, userType, err := o.Token.GetToken(req.Token)
	if err != nil {
		return nil, err
	}
	m, err := o.Database.GetTokens(ctx, userID)
	if err != nil && err != redis.Nil {
		return nil, err
	}
	if len(m) == 0 {
		return nil, eerrs.ErrTokenNotExist.Wrap()
	}
	if _, ok := m[req.Token]; !ok {
		return nil, eerrs.ErrTokenNotExist.Wrap()
	}
	if identity.Required() && userType == 1 {
		if _, err := identity.Validate(ctx, req.Token, userID, "chat", 0); err != nil {
			return nil, eerrs.ErrTokenNotExist.WrapMsg("HRM session unavailable")
		}
	}

	return &adminpb.ParseTokenResp{
		UserID:   userID,
		UserType: userType,
	}, nil
}

func (o *adminServer) GetUserToken(ctx context.Context, req *adminpb.GetUserTokenReq) (*adminpb.GetUserTokenResp, error) {
	tokensMap, err := o.Database.GetTokens(ctx, req.UserID)
	if err != nil {
		return nil, err
	}
	return &adminpb.GetUserTokenResp{TokensMap: tokensMap}, nil
}

func (o *adminServer) InvalidateToken(ctx context.Context, req *adminpb.InvalidateTokenReq) (*adminpb.InvalidateTokenResp, error) {
	// Private RPC metadata extends the fixed protocol without changing its
	// module/version. Public HTTP routes never forward this header. Absence keeps
	// the original all-user revocation for password changes and account bans.
	md, _ := metadata.FromIncomingContext(ctx)
	if tokens := md.Get("openim38-chat-revoke-token"); len(tokens) > 0 {
		if len(tokens) != 1 || tokens[0] == "" {
			return nil, errs.ErrArgs.WrapMsg("one exact Chat credential required")
		}
		userID, err := o.Token.GetTokenForRevocation(tokens[0])
		if err != nil {
			return nil, err
		}
		if userID != req.UserID {
			return nil, errs.ErrNoPermission.WrapMsg("Chat credential identity mismatch")
		}
		if err := o.Database.DeleteTokenValue(ctx, userID, tokens[0]); err != nil && err != redis.Nil {
			return nil, err
		}
		return &adminpb.InvalidateTokenResp{}, nil
	}
	err := o.Database.DeleteToken(ctx, req.UserID)
	if err != nil && err != redis.Nil {
		return nil, err
	}
	log.ZDebug(ctx, "delete token from redis", "userID", req.UserID)
	return &adminpb.InvalidateTokenResp{}, nil
}
