# Search scoring and retention

`recall` combines vector similarity, full-text search, and effective importance:

```text
final_score = normalized_vector * vector_weight
            + normalized_fts * fts_weight
            + effective_importance / 10 * importance_weight
```

The default weights are `0.60`, `0.25`, and `0.15`. Results found by both
vector and full-text search are deduplicated by memory ID.

When `memory.auto_decay` is enabled, importance decays over time and increases
with recall frequency. The defaults apply a 0.88 multiplier every 90 days and
cap the recall boost at 1.0:

```text
decayed   = importance * decay_factor ^ (time_since_access / decay_interval)
boost     = min(access_boost_cap, access_count * access_boost_factor)
effective = clamp(1.0, 10.0, decayed + boost)
```

Maintenance runs when `start_session` is called and is throttled per project by
`maintenance.min_run_interval`. It removes memories whose effective importance
is at or below `maintenance.forget_threshold` and that have been inactive for at
least `maintenance.forget_inactive_days`. With the defaults, auto-forget is only
eligible after two years of inactivity.
