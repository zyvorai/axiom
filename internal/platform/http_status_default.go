//go:build !kubernetes

package platform

// httpStatusFromKnownError: the demo build's provider errors are plain fmt.Errorf
// values, so the message heuristic is all there is to go on (see httpStatusFromMessage).
func httpStatusFromKnownError(err error) (int, bool) {
	return httpStatusFromMessage(err)
}
