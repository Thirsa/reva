// Copyright 2018-2020 CERN
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
//
// In applying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

package ocdav

import (
	"context"
	"net/http"
	"path"

	gatewayv1beta1 "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp/router"
	tokenpkg "github.com/cs3org/reva/pkg/token"
	"github.com/cs3org/reva/pkg/user"
	"google.golang.org/grpc/metadata"
)

// DavHandler routes to the different sub handlers
type DavHandler struct {
	AvatarsHandler     *AvatarsHandler
	FilesHandler       *WebDavHandler
	MetaHandler        *MetaHandler
	TrashbinHandler    *TrashbinHandler
	PublicFilesHandler *WebDavHandler
}

func (h *DavHandler) init(c *Config) error {
	h.AvatarsHandler = new(AvatarsHandler)
	if err := h.AvatarsHandler.init(c); err != nil {
		return err
	}
	h.FilesHandler = new(WebDavHandler)
	if err := h.FilesHandler.init(c.FilesNamespace); err != nil {
		return err
	}
	h.MetaHandler = new(MetaHandler)
	if err := h.MetaHandler.init(c); err != nil {
		return err
	}
	h.TrashbinHandler = new(TrashbinHandler)

	h.PublicFilesHandler = new(WebDavHandler)
	if err := h.PublicFilesHandler.init("public"); err != nil { // jail public file r equests to /public/ prefix
		return err
	}

	return h.TrashbinHandler.init(c)
}

// Handler handles requests
func (h *DavHandler) Handler(s *svc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		var head string
		head, r.URL.Path = router.ShiftPath(r.URL.Path)

		switch head {
		case "avatars":
			h.AvatarsHandler.Handler(s).ServeHTTP(w, r)
		case "files":
			base := path.Join(ctx.Value(ctxKeyBaseURI).(string), "files")
			ctx := context.WithValue(ctx, ctxKeyBaseURI, base)
			r = r.WithContext(ctx)
			h.FilesHandler.Handler(s).ServeHTTP(w, r)
		case "meta":
			base := path.Join(ctx.Value(ctxKeyBaseURI).(string), "meta")
			ctx = context.WithValue(ctx, ctxKeyBaseURI, base)
			r = r.WithContext(ctx)
			h.MetaHandler.Handler(s).ServeHTTP(w, r)
		case "trash-bin":
			base := path.Join(ctx.Value(ctxKeyBaseURI).(string), "trash-bin")
			ctx := context.WithValue(ctx, ctxKeyBaseURI, base)
			r = r.WithContext(ctx)
			h.TrashbinHandler.Handler(s).ServeHTTP(w, r)
		case "public-files":
			base := path.Join(ctx.Value(ctxKeyBaseURI).(string), "public-files")
			ctx = context.WithValue(ctx, ctxKeyBaseURI, base)
			c, err := pool.GetGatewayServiceClient(s.c.GatewaySvc)
			if err != nil {
				w.WriteHeader(http.StatusNotFound)
			}

			_, pass, _ := r.BasicAuth()
			token, _ := router.ShiftPath(r.URL.Path)

			authenticateRequest := gatewayv1beta1.AuthenticateRequest{
				Type:         "publicshares",
				ClientId:     token,
				ClientSecret: pass,
			}

			res, err := c.Authenticate(r.Context(), &authenticateRequest)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if res.Status.Code == rpcv1beta1.Code_CODE_UNAUTHENTICATED {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			ctx = tokenpkg.ContextSetToken(ctx, res.Token)
			ctx = user.ContextSetUser(ctx, res.User)
			ctx = metadata.AppendToOutgoingContext(ctx, tokenpkg.TokenHeader, res.Token)

			r = r.WithContext(ctx)
			h.PublicFilesHandler.Handler(s).ServeHTTP(w, r)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}
