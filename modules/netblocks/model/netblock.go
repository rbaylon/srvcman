// Package netblocksmodel - speedtest netblocks
//
// Each record is one network block in CIDR notation (e.g. 192.168.1.0/24).
// Alongside the database row, every block is kept as a line in the gateway's
// fast.com.blocks file, the file behind pf's <fastdotcom> table. That file
// already carries blocks that ship with the product, so records are added to
// it and removed from it one line at a time - the file is never regenerated
// from the database, which would throw those lines away.
package netblocksmodel

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	Acmd "github.com/rbaylon/arkgatecmd"
	"github.com/rbaylon/srvcman/utils"
	"gorm.io/gorm"
)

const (
	// BlocksFile is the gateway file the blocks are kept in.
	BlocksFile = "/usr/local/arkgate/arkgated/rundir/fast.com.blocks"

	shBin = "/bin/sh"

	// fileTimeout bounds one edit of the blocks file, which is a local
	// file operation on the gateway.
	fileTimeout = 30 * time.Second
)

// Netblock is one network block.
type Netblock struct {
	gorm.Model
	Cidr string `json:"cidr"`
}

// MigrateDB - Create the table if not exist in DB
func MigrateDB(db *gorm.DB) {
	err := db.AutoMigrate(&Netblock{})
	if err != nil {
		log.Fatal(err)
	}
}

// Normalize checks that value is a CIDR block and returns it in canonical
// form: the network address with its prefix length, so "192.168.1.5/24"
// becomes "192.168.1.0/24". Storing one spelling per network is what lets
// duplicates be caught and lines be matched exactly when removing them.
//
// A /0 block is refused: it matches every address, so one stray entry in a
// table that firewall rules act on would apply those rules to all traffic.
func Normalize(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("cidr is required")
	}
	if len(value) > 100 {
		return "", errors.New("cidr exceeds maximum length of 100 characters")
	}
	_, network, err := net.ParseCIDR(value)
	if err != nil {
		return "", errors.New("cidr must be a network in CIDR notation, e.g. 192.168.1.0/24")
	}
	if ones, _ := network.Mask.Size(); ones == 0 {
		return "", errors.New("cidr must not be a /0 block, which matches every address")
	}
	return network.String(), nil
}

// Bind interface as required by go-chi/render
func (n *Netblock) Bind(r *http.Request) error {
	cidr, err := Normalize(n.Cidr)
	if err != nil {
		return err
	}
	n.Cidr = cidr
	return nil
}

// cidrLine is what a canonical CIDR looks like. The file scripts are shell
// text with the block interpolated into them, so this is checked again at
// that point even though Normalize has already run: nothing but these
// characters may ever reach a shell.
var cidrLine = regexp.MustCompile(`^[0-9a-fA-F:.]+/[0-9]{1,3}$`)

// appendScript is the shell text that adds cidr to path as its own line,
// unless an identical line is already there. It makes sure the file ends in
// a newline first: the shipped file has none, and without this the new block
// would be glued onto the last existing one.
func appendScript(path, cidr string) (string, error) {
	if !cidrLine.MatchString(cidr) {
		return "", fmt.Errorf("refusing to write %q to %s", cidr, path)
	}
	return fmt.Sprintf(`f='%[1]s'; c='%[2]s'
[ -f "$f" ] || : > "$f" || exit 1
grep -qxF -- "$c" "$f" && exit 0
[ -z "$(tail -c1 "$f")" ] || echo >> "$f" || exit 1
echo "$c" >> "$f"`, path, cidr), nil
}

// removeScript is the shell text that deletes the lines of path that are
// exactly cidr and leaves every other line alone. The file is rewritten in
// place (cat into it, not mv over it), so its owner and permissions survive.
func removeScript(path, cidr string) (string, error) {
	if !cidrLine.MatchString(cidr) {
		return "", fmt.Errorf("refusing to remove %q from %s", cidr, path)
	}
	return fmt.Sprintf(`f='%[1]s'; c='%[2]s'
[ -f "$f" ] || exit 0
t=$(mktemp) || exit 1
grep -vxF -- "$c" "$f" > "$t"
[ $? -le 1 ] || { rm -f "$t"; exit 1; }
cat "$t" > "$f"; rc=$?
rm -f "$t"
exit $rc`, path, cidr), nil
}

// runScript hands script to arkgated to run as root on the gateway - the
// file is arkgated's, and srvcman may be on another host entirely. It is
// a var so tests can stand in for arkgated.
var runScript = func(name, script string) error {
	conn := utils.GetArkConn()
	if conn == nil {
		return fmt.Errorf("arkgated unreachable")
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(fileTimeout))
	cmd := Acmd.PrepareCmd(name, shBin, []string{"-c", script})
	out, err := cmd.SendCmdOutput(conn)
	if err != nil {
		if out = strings.TrimSpace(out); out != "" {
			return fmt.Errorf("%s: %s", name, out)
		}
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// AddToFile appends cidr to the blocks file.
func AddToFile(cidr string) error {
	s, err := appendScript(BlocksFile, cidr)
	if err != nil {
		return err
	}
	return runScript("NetblockAdd", s)
}

// RemoveFromFile removes cidr from the blocks file.
func RemoveFromFile(cidr string) error {
	s, err := removeScript(BlocksFile, cidr)
	if err != nil {
		return err
	}
	return runScript("NetblockRemove", s)
}

// ErrDuplicate is returned when the block is already recorded.
var ErrDuplicate = errors.New("that network block already exists")

func exists(db *gorm.DB, cidr string, exceptID uint) (bool, error) {
	var count int64
	q := db.Model(&Netblock{}).Where("cidr = ?", cidr)
	if exceptID != 0 {
		q = q.Where("id <> ?", exceptID)
	}
	if err := q.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetAll lists the blocks, oldest first.
func GetAll(db *gorm.DB) ([]Netblock, error) {
	var all []Netblock
	if err := db.Order("id").Find(&all).Error; err != nil {
		return nil, err
	}
	return all, nil
}

// GetById returns one block.
func GetById(db *gorm.DB, id uint) (*Netblock, error) {
	var n Netblock
	if err := db.First(&n, id).Error; err != nil {
		return nil, err
	}
	return &n, nil
}

// Create stores n and appends it to the blocks file. If the file cannot be
// written the row is removed again, so the database and the file never
// disagree about what exists.
func Create(db *gorm.DB, n *Netblock) error {
	if dup, err := exists(db, n.Cidr, 0); err != nil {
		return err
	} else if dup {
		return ErrDuplicate
	}
	if err := db.Create(n).Error; err != nil {
		return err
	}
	if err := AddToFile(n.Cidr); err != nil {
		db.Unscoped().Delete(n)
		return err
	}
	return nil
}

// Update changes the block with n.ID to n.Cidr, in the file as well as the
// database: the new block is added and the old one removed.
func Update(db *gorm.DB, n *Netblock) error {
	old, err := GetById(db, n.ID)
	if err != nil {
		return err
	}
	if old.Cidr == n.Cidr {
		return nil
	}
	if dup, err := exists(db, n.Cidr, n.ID); err != nil {
		return err
	} else if dup {
		return ErrDuplicate
	}
	oldCidr := old.Cidr
	if err := AddToFile(n.Cidr); err != nil {
		return err
	}
	if err := RemoveFromFile(oldCidr); err != nil {
		RemoveFromFile(n.Cidr)
		return err
	}
	old.Cidr = n.Cidr
	if err := db.Save(old).Error; err != nil {
		// Put the file back the way it was.
		AddToFile(oldCidr)
		RemoveFromFile(n.Cidr)
		return err
	}
	*n = *old
	return nil
}

// Delete removes the block from the file, then from the database. The file
// goes first: if it cannot be edited the record stays, instead of leaving a
// block in the file that nothing in the UI can remove.
func Delete(db *gorm.DB, id uint) error {
	old, err := GetById(db, id)
	if err != nil {
		return err
	}
	if err := RemoveFromFile(old.Cidr); err != nil {
		return err
	}
	if err := db.Delete(old).Error; err != nil {
		AddToFile(old.Cidr)
		return err
	}
	return nil
}
