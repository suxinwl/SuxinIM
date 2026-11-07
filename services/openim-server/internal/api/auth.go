// Copyright © 2023 OpenIM. All rights reserved.
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

package api

import (
	"github.com/gin-gonic/gin"
	"github.com/openimsdk/protocol/auth"
	"github.com/openimsdk/tools/a2r"
	"github.com/openimsdk/tools/apiresp"
	"github.com/openimsdk/tools/errs"
	"openim38.local/identity"
)

type AuthApi struct {
	Client auth.AuthClient
}

func NewAuthApi(client auth.AuthClient) AuthApi {
	return AuthApi{client}
}

func (o *AuthApi) GetAdminToken(c *gin.Context) {
	a2r.Call(c, auth.AuthClient.GetAdminToken, o.Client)
}

func (o *AuthApi) GetUserToken(c *gin.Context) {
	a2r.Call(c, auth.AuthClient.GetUserToken, o.Client)
}

func (o *AuthApi) ParseToken(c *gin.Context) {
	bindRequestEntry(c)
	var req auth.ParseTokenReq
	if err := c.ShouldBindJSON(&req); err != nil {
		apiresp.GinError(c, errs.ErrArgs.WrapMsg("invalid token request"))
		return
	}
	resp, err := o.Client.ParseToken(c, &req)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	if identity.Required() && identity.Managed(resp.UserID) {
		v, e := identity.Validate(c.Request.Context(), req.Token, resp.UserID, "im", int(resp.PlatformID))
		ip, ipErr := identity.ClientIP(c.Request)
		if e != nil || ipErr != nil || identity.CheckSessionAccess(c.Request.Context(), v, ip, "connect", "") != nil {
			apiresp.GinError(c, errs.ErrNoPermission.WrapMsg("HRM access denied"))
			return
		}
	}
	apiresp.GinSuccess(c, resp)
}

func (o *AuthApi) ForceLogout(c *gin.Context) {
	a2r.Call(c, auth.AuthClient.ForceLogout, o.Client)
}
