# splint

[![Build Status](https://travis-ci.org/agflow/splint.svg?branch=master)](https://travis-ci.org/agflow/splint)
[![GoDoc](https://godoc.org/github.com/agflow/splint?status.svg)](https://godoc.org/github.com/agflow/splint)
[![Go Report](https://goreportcard.com/badge/github.com/agflow/splint)](https://goreportcard.com/report/github.com/agflow/splint)
[![Coverage](https://codecov.io/gh/agflow/splint/branch/master/graph/badge.svg)](https://codecov.io/gh/agflow/splint)

`splint` is a little Go application to analyze Go source files.  It finds any functions that are
too long or have too many parameters or results.

These are typical signs that a function is doing too much.  We find `splint` to be a helpful tool
for detecting potential problem areas in our code, areas that should be refactored.  We tolerate long
functions and functions with long parameter/result lists when they are needed, but generally try to
keep them short.

## Installation

Use `go install`:

    go install github.com/agflow/splint

## About

This is a fork of [splint](https://github.com/stathat/splint).
