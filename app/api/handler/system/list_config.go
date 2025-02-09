// Copyright 2023 Harness, Inc.
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

package system

import (
	"net/http"

	"github.com/harness/gitness/app/api/controller/system"
	"github.com/harness/gitness/app/api/render"
)

type ConfigOutput struct {
	UserSignupAllowed bool `json:"user_signup_allowed"`
}

// HandleGetConfig returns an http.HandlerFunc that processes an http.Request
// and returns a struct containing all system configs exposed to the users.
func HandleGetConfig(sysCtrl *system.Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		userSignupAllowed, err := sysCtrl.IsUserSignupAllowed(ctx)
		if err != nil {
			render.TranslatedUserError(w, err)
			return
		}
		render.JSON(w, http.StatusOK, ConfigOutput{
			UserSignupAllowed: userSignupAllowed,
		})
	}
}
