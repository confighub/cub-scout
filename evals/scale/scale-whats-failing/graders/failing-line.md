---
type: regex
pattern: '^FAILING:(?=[^\n]*\bteam\-10\/cron\b)(?=[^\n]*\bteam\-15\/api\b)[^,\n]+(?:,[^,\n]+){1}[ \t]*$'
flags: im
target: last_message
---
