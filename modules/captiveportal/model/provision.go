package captiveportalmodel

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"

	groupmodel "github.com/rbaylon/srvcman/modules/groups/model"
	policymodel "github.com/rbaylon/srvcman/modules/policies/model"
	usermodel "github.com/rbaylon/srvcman/modules/users/model"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// Names of the records provisioned for the portal. They are fixed rather
// than generated so that re-saving the config repairs the same three
// records instead of accumulating a new set each time.
const (
	ApiUsername = "captiveportal"
	ApiGroup    = "captiveportal"
	ApiPolicy   = "CaptivePortalVouchersRO"
)

// The portal only ever reads vouchers (it validates a code against
// vouchers/value/...), so its policy is read-only on that module. RBAC
// here is a group join: a policy applies to a user when the two share a
// group, so provisioning means a user, a group, a policy, and the two
// links between them.
const (
	apiPolicyType     = "vouchers"
	apiPolicyResource = "any"
	apiPolicyAction   = "RO"
)

// randomPassword returns a 32-character hex secret. Hex rather than
// base64 so the value survives being embedded in a "user:password" pair
// without any character that would need escaping.
func randomPassword() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// BasicAuth encodes credentials the way an Authorization: Basic header
// carries them, which is the form the portal's API_AUTH holds.
func BasicAuth(username, password string) string {
	return base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
}

// EnsureApiUser makes sure the portal's API account exists with read-only
// access to the vouchers module, and returns the base64 credential its
// .env needs.
//
// existing is the credential already stored on the config record. When it
// is present and the account still exists, it is returned unchanged:
// srvcman only ever holds the bcrypt hash of the password, so the
// plaintext cannot be recovered from the user record, and minting a new
// one on every save would invalidate the credential the running portal is
// still using. A new password is generated only when there is nothing
// usable to keep - first save, or the account having been deleted by hand.
func EnsureApiUser(db *gorm.DB, existing string) (string, error) {
	group, err := ensureGroup(db)
	if err != nil {
		return "", err
	}
	if err := ensurePolicy(db, group); err != nil {
		return "", err
	}

	var user usermodel.User
	found := db.Preload("Groups").Where("Username = ?", ApiUsername).First(&user).Error == nil
	if found && existing != "" {
		// Account and credential both intact; just make sure the group
		// link is still there, since that is what the policy hangs off.
		if err := ensureUserInGroup(db, &user, group); err != nil {
			return "", err
		}
		return existing, nil
	}

	password, err := randomPassword()
	if err != nil {
		return "", err
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	if err != nil {
		return "", err
	}
	user.Username = ApiUsername
	user.Password = string(hashed)
	user.Firstname = "Captive"
	user.Lastname = "Portal"
	if found {
		if err := db.Save(&user).Error; err != nil {
			return "", fmt.Errorf("updating %s api user: %w", ApiUsername, err)
		}
		log.Println("captiveportal: reset the api user's password")
	} else {
		if err := db.Create(&user).Error; err != nil {
			return "", fmt.Errorf("creating %s api user: %w", ApiUsername, err)
		}
		log.Println("captiveportal: created the api user")
	}
	if err := ensureUserInGroup(db, &user, group); err != nil {
		return "", err
	}
	return BasicAuth(ApiUsername, password), nil
}

func ensureGroup(db *gorm.DB) (*groupmodel.Group, error) {
	var group groupmodel.Group
	if err := db.Where("Groupname = ?", ApiGroup).First(&group).Error; err == nil {
		return &group, nil
	}
	group.Groupname = ApiGroup
	if err := db.Create(&group).Error; err != nil {
		return nil, fmt.Errorf("creating %s group: %w", ApiGroup, err)
	}
	return &group, nil
}

// ensurePolicy creates the read-only vouchers policy if it is missing and
// makes sure the portal's group is attached to it. An existing policy's
// Type/Resource/Action are left alone - an admin may have deliberately
// narrowed them, and silently widening access back out on every config
// save would be the wrong way to lose that edit.
func ensurePolicy(db *gorm.DB, group *groupmodel.Group) error {
	var policy policymodel.Policy
	if err := db.Preload("Groups").Where("Name = ?", ApiPolicy).First(&policy).Error; err != nil {
		policy = policymodel.Policy{
			Name:     ApiPolicy,
			Type:     apiPolicyType,
			Resource: apiPolicyResource,
			Action:   apiPolicyAction,
			Groups:   []groupmodel.Group{*group},
		}
		if err := db.Create(&policy).Error; err != nil {
			return fmt.Errorf("creating %s policy: %w", ApiPolicy, err)
		}
		return nil
	}
	for _, g := range policy.Groups {
		if g.Groupname == ApiGroup {
			return nil
		}
	}
	if err := db.Model(&policy).Association("Groups").Append(group); err != nil {
		return fmt.Errorf("attaching %s group to %s policy: %w", ApiGroup, ApiPolicy, err)
	}
	return nil
}

func ensureUserInGroup(db *gorm.DB, user *usermodel.User, group *groupmodel.Group) error {
	for _, g := range user.Groups {
		if g.Groupname == ApiGroup {
			return nil
		}
	}
	if err := db.Model(user).Association("Groups").Append(group); err != nil {
		return fmt.Errorf("attaching %s group to %s user: %w", ApiGroup, ApiUsername, err)
	}
	return nil
}
