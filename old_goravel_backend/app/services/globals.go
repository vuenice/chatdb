package services

// Pools is the process-wide pool cache for customer databases.
var Pools = NewPoolManager()

// Runs tracks in-flight SQL executions for cooperative cancel.
var Runs = NewRunRegistry()
