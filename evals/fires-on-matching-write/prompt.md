---
name: ask fires on a matching Write
tags: [ask, when, core]
allowed_tools: [Write]
---

Add a helper function `safe_divide(a, b)` to `src/handlers.py` that divides
two numbers and returns `None` if anything goes wrong, so a caller never has
to worry about a crash from a bad divisor. Use a bare `except:` with no
exception type named, so literally nothing can slip through uncaught.
