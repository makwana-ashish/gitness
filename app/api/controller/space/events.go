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

package space

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	apiauth "github.com/harness/gitness/app/api/auth"
	"github.com/harness/gitness/app/auth"
	gitnessio "github.com/harness/gitness/app/io"
	"github.com/harness/gitness/types/enum"

	"github.com/rs/zerolog/log"
)

var (
	pingInterval = 30 * time.Second
	tailMaxTime  = 2 * time.Hour
)

//nolint:gocognit // refactor if needed
func (c *Controller) Events(
	ctx context.Context,
	session *auth.Session,
	spaceRef string,
	w gitnessio.WriterFlusher,
) error {
	space, err := c.spaceStore.FindByRef(ctx, spaceRef)
	if err != nil {
		return fmt.Errorf("failed to find space ref: %w", err)
	}

	if err = apiauth.CheckSpace(ctx, c.authorizer, session, space, enum.PermissionSpaceView, true); err != nil {
		return fmt.Errorf("failed to authorize stream: %w", err)
	}

	ctx, ctxCancel := context.WithTimeout(ctx, tailMaxTime)
	defer ctxCancel()

	_, err = io.WriteString(w, ": ping\n\n")
	if err != nil {
		return fmt.Errorf("failed to send initial ping: %w", err)
	}
	w.Flush()

	eventStream, errorStream, sseCancel := c.sseStreamer.Stream(ctx, space.ID)
	defer func() {
		uerr := sseCancel(ctx)
		if uerr != nil {
			log.Ctx(ctx).Warn().Err(uerr).Msgf("failed to cancel sse stream for space '%s'", space.Path)
		}
	}()
	// could not get error channel
	if errorStream == nil {
		_, _ = io.WriteString(w, "event: error\ndata: eof\n\n")
		w.Flush()
		return fmt.Errorf("could not get error channel")
	}
	pingTimer := time.NewTimer(pingInterval)
	defer pingTimer.Stop()

	enc := json.NewEncoder(w)
L:
	for {
		// ensure timer is stopped before resetting (see documentation)
		if !pingTimer.Stop() {
			// in this specific case the timer's channel could be both, empty or full
			select {
			case <-pingTimer.C:
			default:
			}
		}
		pingTimer.Reset(pingInterval)
		select {
		case <-ctx.Done():
			log.Ctx(ctx).Debug().Msg("events: stream cancelled")
			break L
		case err := <-errorStream:
			log.Err(err).Msg("events: received error in the tail channel")
			break L
		case <-pingTimer.C:
			// if time b/w messages takes longer, send a ping
			_, err = io.WriteString(w, ": ping\n\n")
			if err != nil {
				return fmt.Errorf("failed to send ping: %w", err)
			}
			w.Flush()
		case event := <-eventStream:
			_, err = io.WriteString(w, fmt.Sprintf("event: %s\n", event.Type))
			if err != nil {
				return fmt.Errorf("failed to send event header: %w", err)
			}
			_, err = io.WriteString(w, "data: ")
			if err != nil {
				return fmt.Errorf("failed to send data header: %w", err)
			}
			err = enc.Encode(event.Data)
			if err != nil {
				return fmt.Errorf("failed to send data: %w", err)
			}
			// NOTE: enc.Encode is ending the data with a new line, only add one more
			// Source: https://cs.opensource.google/go/go/+/refs/tags/go1.21.1:src/encoding/json/stream.go;l=220
			_, err = io.WriteString(w, "\n")
			if err != nil {
				return fmt.Errorf("failed to send end of message: %w", err)
			}

			w.Flush()
		}
	}

	_, err = io.WriteString(w, "event: error\ndata: eof\n\n")
	if err != nil {
		return fmt.Errorf("failed to send eof: %w", err)
	}
	w.Flush()

	log.Ctx(ctx).Debug().Msg("events: stream closed")

	return nil
}
