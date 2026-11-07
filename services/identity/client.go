package identity

// LogicalClient is a policy category, never a device attestation. A browser's
// UA cannot turn it into proof of an operating system or a trusted native app.
func LogicalClient(platform, policy int, client string) (string, error) {
	if client == "" {
		switch platform {
		case 1:
			client = "ios"
		case 2:
			client = "android"
		case 3:
			client = "pc"
		case 5:
			client = "web_desktop"
			if policy == 2 {
				client = "web_mobile"
			}
		}
	}
	valid := platform == 1 && client == "ios" || platform == 2 && client == "android" || platform == 3 && client == "pc" || platform == 5 && (client == "web_desktop" || client == "web_mobile")
	if !valid {
		return "", ErrSession
	}
	return client, nil
}

func SessionClient(v Session) (string, error) {
	return LogicalClient(v.Platform, v.PolicyPlatform, v.LogicalClient)
}
