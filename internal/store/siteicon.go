package store

import "timestamps/internal/iconcolor"

// SiteIcon returns the stored icon of a website (nil when none was found)
// and when it was looked up; found is false if it never was.
func (db *DB) SiteIcon(domain string) (icon []byte, checked int64, found bool) {
	err := db.sql.QueryRow("SELECT icon, checked FROM site_icon WHERE domain = ?", domain).Scan(&icon, &checked)
	return icon, checked, err == nil
}

// SetSiteIcon stores the icon of a website (nil: none was found) and the
// color picked from it.
func (db *DB) SetSiteIcon(domain string, icon []byte, checked int64) {
	color, _ := iconcolor.FromBytes(icon)
	db.sql.Exec(`INSERT INTO site_icon(domain, icon, checked, color) VALUES(?, ?, ?, ?)
		ON CONFLICT(domain) DO UPDATE SET icon = excluded.icon, checked = excluded.checked, color = excluded.color`, domain, icon, checked, color)
	if color != "" {
		db.touch()
	}
}

// SiteColors returns the colors picked from website icons, by domain.
func (db *DB) SiteColors() map[string]string {
	out := map[string]string{}
	rows, err := db.sql.Query("SELECT domain, color FROM site_icon WHERE color != ''")
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var d, c string
		rows.Scan(&d, &c)
		out[d] = c
	}
	return out
}
