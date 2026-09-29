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
// than generated so that re-saving the config repairs the same records
// instead of accumulating a new set each time.
const (
	ApiUsername = "captiveportal"
	ApiGroup    = "captiveportal"
)

// apiPolicies are the modules the portal needs to reach, each read-only.
// RBAC here is a group join: a policy applies to a user when the two share
// a group, so provisioning means a user, a group, these policies, and the
// links between them.
//
// Both entries are needed by different halves of the app:
//
//   - vouchers: the request path, validating a code against
//     vouchers/value/<code>/<ip>/<routerid>.
//   - runtimes: the background PfReloader, which polls
//     runtime/query/updatepf/<ROUTER_INDEX> and then acknowledges with
//     runtime/delete/<ROUTER_INDEX>. Both of those are GET routes despite
//     what "delete" suggests, so read-only covers them - RO permits GET
//     and nothing else (see security.validateToken).
//
// Resource is "any" rather than the specific paths because resource
// matching is an exact string compare, and these paths carry the voucher
// code and router index as paths segments - there is no fixed string to
// match against.
var apiPolicies = []struct {
	Name string
	Type string
}{
	{"CaptivePortalVouchersRO", "vouchers"},
	{"CaptivePortalRuntimesRO", "runtimes"},
}

const (
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
// access to the modules it calls (see apiPolicies), and returns the base64
// credential its .env needs.
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
	if err := ensurePolicies(db, group); err != nil {
		return "", err
	}

	var user usermodel.User
	// Unscoped: Username carries a plain unique constraint, and a default
	// (scoped) lookup excludes soft-deleted rows - so if this account was
	// ever deleted through the ordinary Users UI, a scoped First() would
	// report not-found and the Create() below would collide with the
	// still-present deleted row instead of reviving it. revive clears
	// DeletedAt when that happens; Password/Groups are untouched by a soft
	// delete, so a revived account's existing hash still verifies.
	found := db.Unscoped().Preload("Groups").Where("Username = ?", ApiUsername).First(&user).Error == nil
	if found && user.DeletedAt.Valid {
		if err := db.Unscoped().Model(&user).Update("deleted_at", nil).Error; err != nil {
			return "", fmt.Errorf("reviving %s api user: %w", ApiUsername, err)
		}
		user.DeletedAt.Valid = false
		log.Println("captiveportal: revived the api user, which had been deleted")
	}
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

// ensureGroup finds or creates the portal's group. The lookup is
// Unscoped for the same reason the user lookup in EnsureApiUser is:
// Groupname carries a plain unique constraint, so a group deleted through
// the ordinary Groups UI leaves a row behind that a scoped First() cannot
// see but Create() would still collide with.
func ensureGroup(db *gorm.DB) (*groupmodel.Group, error) {
	var group groupmodel.Group
	if err := db.Unscoped().Where("Groupname = ?", ApiGroup).First(&group).Error; err == nil {
		if group.DeletedAt.Valid {
			if err := db.Unscoped().Model(&group).Update("deleted_at", nil).Error; err != nil {
				return nil, fmt.Errorf("reviving %s group: %w", ApiGroup, err)
			}
			group.DeletedAt.Valid = false
			log.Println("captiveportal: revived the", ApiGroup, "group, which had been deleted")
		}
		return &group, nil
	}
	group.Groupname = ApiGroup
	if err := db.Create(&group).Error; err != nil {
		return nil, fmt.Errorf("creating %s group: %w", ApiGroup, err)
	}
	return &group, nil
}

// ensurePolicies creates each missing read-only policy and makes sure the
// portal's group is attached to it. An existing policy's
// Type/Resource/Action are left alone - an admin may have deliberately
// narrowed them, and silently widening access back out on every config
// save would be the wrong way to lose that edit.
//
// Because this runs on every save, an install that predates one of these
// policies picks it up the next time the config is saved.
//
// The lookup is Unscoped for the same reason ensureGroup's is: Name
// carries a plain unique constraint, so a policy deleted through the
// ordinary Policies UI leaves a row behind that a scoped First() cannot
// see but Create() would still collide with.
func ensurePolicies(db *gorm.DB, group *groupmodel.Group) error {
	for _, spec := range apiPolicies {
		var policy policymodel.Policy
		if err := db.Unscoped().Preload("Groups").Where("Name = ?", spec.Name).First(&policy).Error; err != nil {
			policy = policymodel.Policy{
				Name:     spec.Name,
				Type:     spec.Type,
				Resource: apiPolicyResource,
				Action:   apiPolicyAction,
				Groups:   []groupmodel.Group{*group},
			}
			if err := db.Create(&policy).Error; err != nil {
				return fmt.Errorf("creating %s policy: %w", spec.Name, err)
			}
			log.Println("captiveportal: created the", spec.Name, "policy")
			continue
		}
		if policy.DeletedAt.Valid {
			if err := db.Unscoped().Model(&policy).Update("deleted_at", nil).Error; err != nil {
				return fmt.Errorf("reviving %s policy: %w", spec.Name, err)
			}
			policy.DeletedAt.Valid = false
			log.Println("captiveportal: revived the", spec.Name, "policy, which had been deleted")
		}
		attached := false
		for _, g := range policy.Groups {
			if g.Groupname == ApiGroup {
				attached = true
				break
			}
		}
		if attached {
			continue
		}
		if err := db.Model(&policy).Association("Groups").Append(group); err != nil {
			return fmt.Errorf("attaching %s group to %s policy: %w", ApiGroup, spec.Name, err)
		}
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
