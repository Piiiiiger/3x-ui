package xray

import (
	"encoding/json"
	"fmt"
)

// ApplyHotDiff reconciles a running core with diff through its gRPC API. Removals
// go first so changed handlers and port swaps never collide with the additions.
func ApplyHotDiff(api *XrayAPI, diff *HotDiff) error {
	for _, u := range diff.RemovedUsers {
		if err := api.RemoveUser(u.Tag, u.Email); err != nil && !IsMissingHandlerErr(err) {
			return fmt.Errorf("remove user [%s] from [%s]: %w", u.Email, u.Tag, err)
		}
	}
	for _, tag := range diff.RemovedInboundTags {
		if err := api.DelInbound(tag); err != nil && !IsMissingHandlerErr(err) {
			return fmt.Errorf("remove inbound [%s]: %w", tag, err)
		}
	}
	for _, tag := range diff.RemovedOutboundTags {
		if err := api.DelOutbound(tag); err != nil && !IsMissingHandlerErr(err) {
			return fmt.Errorf("remove outbound [%s]: %w", tag, err)
		}
	}
	for _, ob := range diff.AddedOutbounds {
		if err := addOutboundReconciling(api, ob); err != nil {
			return fmt.Errorf("add outbound: %w", err)
		}
	}
	for _, ib := range diff.AddedInbounds {
		if err := addInboundReconciling(api, ib); err != nil {
			return fmt.Errorf("add inbound: %w", err)
		}
	}
	for _, u := range diff.AddedUsers {
		if err := addUserReconciling(api, u); err != nil {
			return fmt.Errorf("add user [%s] to [%s]: %w", u.Email, u.Tag, err)
		}
	}
	if diff.RoutingConfig != nil {
		if err := api.ApplyRoutingConfig(diff.RoutingConfig); err != nil {
			return fmt.Errorf("apply routing config: %w", err)
		}
	}
	return nil
}

// addUserReconciling adds a user, and on an email conflict (the user was
// already applied through the runtime API) replaces the existing user instead.
func addUserReconciling(api *XrayAPI, u UserOp) error {
	err := api.AddUser(u.Protocol, u.Tag, u.User)
	if err == nil || !IsUserExistsErr(err) {
		return err
	}
	if delErr := api.RemoveUser(u.Tag, u.Email); delErr != nil && !IsMissingHandlerErr(delErr) {
		return delErr
	}
	return api.AddUser(u.Protocol, u.Tag, u.User)
}

// addInboundReconciling adds an inbound, and on a tag conflict (the handler
// was already created through the runtime API while the stored snapshot was
// stale) replaces the existing handler instead.
func addInboundReconciling(api *XrayAPI, inbound []byte) error {
	err := api.AddInbound(inbound)
	if err == nil || !IsExistingTagErr(err) {
		return err
	}
	var meta struct {
		Tag string `json:"tag"`
	}
	if jsonErr := json.Unmarshal(inbound, &meta); jsonErr != nil || meta.Tag == "" {
		return err
	}
	if delErr := api.DelInbound(meta.Tag); delErr != nil && !IsMissingHandlerErr(delErr) {
		return delErr
	}
	return api.AddInbound(inbound)
}

// addOutboundReconciling mirrors addInboundReconciling for outbounds.
func addOutboundReconciling(api *XrayAPI, outbound []byte) error {
	err := api.AddOutbound(outbound)
	if err == nil || !IsExistingTagErr(err) {
		return err
	}
	var meta struct {
		Tag string `json:"tag"`
	}
	if jsonErr := json.Unmarshal(outbound, &meta); jsonErr != nil || meta.Tag == "" {
		return err
	}
	if delErr := api.DelOutbound(meta.Tag); delErr != nil && !IsMissingHandlerErr(delErr) {
		return delErr
	}
	return api.AddOutbound(outbound)
}
