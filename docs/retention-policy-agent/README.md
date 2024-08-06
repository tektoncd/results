<!--

---
linkTitle: "Results Retention Policy Agent"
weight: 2
---

-->

# Result Retention Policy Agent

The Results Retention Policy Agent removes older Results and their associated Records from the DB.

## Configuration
The Results Retention Policy Agent config uses following field

- `runAt`: determines when to run pruning job for DB.
- `maxRetention`: for how long to keep results and records.
