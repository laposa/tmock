package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/laposa/tmock/app-go/internal/logx"
)

// Actor is who makes a change, for the tmock.entity.changed event (spec §2.4.2). The REST
// API and the UI pass the logged-in user.
type Actor struct {
	UserID int64
}

// UserActor is a change made by the logged-in user id.
func UserActor(id int64) Actor { return Actor{UserID: id} }

// entityChanged logs tmock.entity.changed. It is not an audit log: it carries no
// before/after values.
func (a *App) entityChanged(ctx context.Context, actor Actor, entity string, id int64, action string, extra ...slog.Attr) {
	attrs := append([]slog.Attr{
		slog.String(logx.AttrEventName, logx.EventEntityChanged),
		slog.String(logx.AttrEntityType, entity),
		slog.Int64(logx.AttrEntityID, id),
		slog.String(logx.AttrEntityAction, action),
		slog.String(logx.AttrActor, logx.ActorUser),
		slog.Int64(logx.AttrUserID, actor.UserID),
	}, extra...)
	a.logger.LogAttrs(ctx, slog.LevelInfo, fmt.Sprintf("%s %d: %s", entity, id, action), attrs...)
}
