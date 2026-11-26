package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/laposa/tmock/app-go/internal/logx"
)

// Actor is who makes a change, for the tmock.entity.changed event (spec §2.4.2): the
// logged-in user, or a client through its token (spec §6.3).
type Actor struct {
	UserID   int64
	ClientID int64 // set for a change made with a client token
}

// UserActor is a change made by the logged-in user id.
func UserActor(id int64) Actor { return Actor{UserID: id} }

// ClientTokenActor is a change made with the token of client id.
func ClientTokenActor(id int64) Actor { return Actor{ClientID: id} }

// attrs are the tmock.actor attribute and the actor's id.
func (a Actor) attrs() []slog.Attr {
	if a.ClientID != 0 {
		return []slog.Attr{slog.String(logx.AttrActor, logx.ActorClientToken), slog.Int64(logx.AttrClientID, a.ClientID)}
	}
	return []slog.Attr{slog.String(logx.AttrActor, logx.ActorUser), slog.Int64(logx.AttrUserID, a.UserID)}
}

// entityChanged logs tmock.entity.changed. It is not an audit log: it carries no
// before/after values.
func (a *App) entityChanged(ctx context.Context, actor Actor, entity string, id int64, action string, extra ...slog.Attr) {
	attrs := append([]slog.Attr{
		slog.String(logx.AttrEventName, logx.EventEntityChanged),
		slog.String(logx.AttrEntityType, entity),
		slog.Int64(logx.AttrEntityID, id),
		slog.String(logx.AttrEntityAction, action),
	}, actor.attrs()...)
	attrs = append(attrs, extra...)
	a.logger.LogAttrs(ctx, slog.LevelInfo, fmt.Sprintf("%s %d: %s", entity, id, action), attrs...)
}
