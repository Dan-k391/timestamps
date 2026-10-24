package store

// SiteIcon returns the stored icon of a website (nil when none was found)
// and when it was looked up; found is false if it never was.
func (db *DB) SiteIcon(domain string) (icon []byte, checked int64, found bool) {
	err := db.sql.QueryRow("SELECT icon, checked FROM site_icon WHERE domain = ?", domain).Scan(&icon, &checked)
	return icon, checked, err == nil
}

// SetSiteIcon stores the icon of a website (nil: none was found).
func (db *DB) SetSiteIcon(domain string, icon []byte, checked int64) {
	db.sql.Exec(`INSERT INTO site_icon(domain, icon, checked) VALUES(?, ?, ?)
		ON CONFLICT(domain) DO UPDATE SET icon = excluded.icon, checked = excluded.checked`, domain, icon, checked)
}
