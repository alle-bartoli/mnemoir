# Search scoring and retention

`recall` combines vector similarity, full-text search, and effective importance:

```text
final_score = normalized_vector * vector_weight
            + normalized_fts * fts_weight
            + effective_importance / 10 * importance_weight
```

The default weights are `0.60`, `0.25`, and `0.15`. Results found by both vector and full-text search are deduplicated by memory ID.

Importance decays over time and increases with recall frequency:

```text
decayed   = importance * decay_factor ^ (time_since_access / decay_interval)
boost     = min(access_boost_cap, access_count * access_boost_factor)
effective = clamp(1.0, 10.0, decayed + boost)
```

Maintenance runs periodically and removes memories below `maintenance.forget_threshold` that have been inactive for `maintenance.forget_inactive_days`. 
Both thresholds are configurable.
