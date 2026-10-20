# GoLog
Custom logging packages to use in golang

## Usage

```go
GoLog.ToFile()                          // logs/<executable>/<date>.log, new file every day at 00:01
GoLog.SetRetention(30 * 24 * time.Hour) // delete older log files now and after every daily switch
GoLog.SetPrivacy(true)                  // shorten marked IPs, redact marked personal data

GoLog.Warnf("login failed from %s for %s", GoLog.IP(addr), GoLog.Personal(user))
// privacy on:  login failed from 203.0.113.0 for [redacted]
// privacy off: login failed from 203.0.113.77 for alice
```

`GoLog.IP` keeps the /24 of an IPv4 address and the /48 of an IPv6 address. Only marked values are changed: text written through `GoLog.Writer()`, which appends raw lines to the current log file, is not. Failed deletions of old logs are logged as errors. Console output goes wherever the process writes stdout, for example journald, where the retention does not apply.
