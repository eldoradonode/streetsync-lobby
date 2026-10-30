package main

import (
	"database/sql"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

// Connection pool settings for the PostgreSQL pool (the lib/pq driver and sql.Open are unchanged).
//
//	DB_MAX_OPEN_CONNS        most connections this instance opens at once (default 4, allowed 1 to 100)
//	DB_MAX_IDLE_CONNS        connections kept open while idle (default 2, never more than the open limit)
//	DB_CONN_MAX_LIFETIME_MIN minutes before a connection is replaced (default 30)
//	DB_CONN_MAX_IDLE_MIN     minutes an idle connection is kept (default 10, 0 keeps them until the lifetime ends)
//	PRUNE_BATCH              rows per cleanup DELETE statement (default 1000, allowed 10 to 100000)
//
// Budget: every instance can open DB_MAX_OPEN_CONNS connections, so
// instances x DB_MAX_OPEN_CONNS + migrations + admin sessions must stay under the database or pooler limit.
type poolConfig struct {
	MaxOpen     int
	MaxIdle     int
	MaxLifetime time.Duration
	MaxIdleTime time.Duration
}

func defaultPool() poolConfig {
	return poolConfig{MaxOpen: 4, MaxIdle: 2, MaxLifetime: 30 * time.Minute, MaxIdleTime: 10 * time.Minute}
}

// envRange reads an integer setting. Anything that is not a whole number in [min, max] is ignored with a log line.
func envRange(name string, def, min, max int) int {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < min || n > max {
		log.Printf("ignoring %s=%q (allowed %d to %d), using %d", name, v, min, max, def)
		return def
	}
	return n
}

func poolFromEnv() poolConfig {
	d := defaultPool()
	c := poolConfig{
		MaxOpen:     envRange("DB_MAX_OPEN_CONNS", d.MaxOpen, 1, 100),
		MaxIdle:     envRange("DB_MAX_IDLE_CONNS", d.MaxIdle, 0, 100),
		MaxLifetime: time.Duration(envRange("DB_CONN_MAX_LIFETIME_MIN", int(d.MaxLifetime/time.Minute), 1, 24*60)) * time.Minute,
		MaxIdleTime: time.Duration(envRange("DB_CONN_MAX_IDLE_MIN", int(d.MaxIdleTime/time.Minute), 0, 24*60)) * time.Minute,
	}
	return c.normalize()
}

// normalize keeps the idle count within the open limit, which database/sql would do silently anyway.
func (c poolConfig) normalize() poolConfig {
	if c.MaxOpen < 1 {
		c.MaxOpen = 1
	}
	if c.MaxIdle > c.MaxOpen {
		c.MaxIdle = c.MaxOpen
	}
	return c
}

func (c poolConfig) apply(db *sql.DB) {
	c = c.normalize()
	db.SetMaxOpenConns(c.MaxOpen)
	db.SetMaxIdleConns(c.MaxIdle)
	db.SetConnMaxLifetime(c.MaxLifetime)
	db.SetConnMaxIdleTime(c.MaxIdleTime)
}
