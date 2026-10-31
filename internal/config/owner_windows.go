//go:build windows

package config

import "golang.org/x/sys/windows"

// ownedByCurrentUser tells whether the file belongs to the user running pt (or to the
// administrators, who own files created in an elevated shell). Other users could plant a
// .pipetower file e.g. at the root of a data drive, where everybody may create files.
func ownedByCurrentUser(path string) bool {
	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return false
	}
	owner, _, err := descriptor.Owner()
	if err != nil || owner == nil {
		return false
	}

	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return false
	}
	administrators, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)

	return owner.Equals(user.User.Sid) || (err == nil && owner.Equals(administrators))
}
