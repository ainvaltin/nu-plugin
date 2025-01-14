package nu

/*
Nu EnterForeground engine call says:

On Unix-like operating systems, if the response is Value pipeline data, it
contains an Int which is the process group ID the plugin must join using
setpgid() in order to be in the foreground.

ie it seems that on non-Unix systems we should do nothing.
*/
func enterForeground(_ int64) error {
	return nil
}
