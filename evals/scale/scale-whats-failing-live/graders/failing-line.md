---
type: regex
pattern: '^FAILING:(?=[^\n]*\bteam\-10/cron[ \t]*(?:,|$))(?=[^\n]*\bteam\-15/api[ \t]*(?:,|$))[ \t]*(?:team\-10/cron|team\-15/api)(?:[ \t]*,[ \t]*(?:team\-10/cron|team\-15/api)){1}[ \t]*$'
flags: im
target: last_message
---
