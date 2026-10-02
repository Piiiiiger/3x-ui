package service

import "strings"

// SetComment changes only a client's remark. It saves through Update, the path a
// plan renewal takes, so the inbounds' copies of the client follow.
func (s *ClientService) SetComment(inboundSvc *InboundService, email, comment string) (bool, error) {
	rec, err := s.GetRecordByEmail(nil, email)
	if err != nil {
		return false, err
	}
	client := rec.ToClient()
	client.Comment = strings.TrimSpace(comment)
	return s.Update(inboundSvc, rec.Id, *client, rec.LimitHwid)
}
