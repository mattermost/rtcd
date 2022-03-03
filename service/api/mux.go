// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package api

import (
	"net/http"
)

type Handler func(http.ResponseWriter, *http.Request)

func (s *Server) RegisterHandler(path string, handler Handler) {
	s.mux.HandleFunc(path, handler)
}
