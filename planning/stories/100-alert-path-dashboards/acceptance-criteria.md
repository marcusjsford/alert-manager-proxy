# 100 — Acceptance criteria

- [ ] Every panel's query runs against real data and returns results (screenshot or result table in
      `findings.md`).
- [ ] The "last seen v1 per client" table matches a manual count from raw spans for one day.
- [ ] The "client went silent" rule fires in a test where a synthetic client stops posting, and does
      not fire for a client whose interval is longer than the window (window choice recorded).
- [ ] Dashboard JSON and rules are in the repo and import cleanly into a fresh instance.
