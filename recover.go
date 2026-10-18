package main

import "log"

// safeGo runs f in a goroutine and logs a panic instead of crashing the server.
func safeGo(f func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("recovered panic: %v", r)
			}
		}()
		f()
	}()
}
