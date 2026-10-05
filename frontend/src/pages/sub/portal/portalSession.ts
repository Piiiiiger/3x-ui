// Thrown by the portal's polled fetches on a 401, so the page returns to the
// sign-in form instead of retrying with a dead session.
export class PortalSessionEnded extends Error {}
