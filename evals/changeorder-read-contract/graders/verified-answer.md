type: regex
pattern: "^\\s*\\{\\s*\"selected_space\"\\s*:\\s*\"prod\"\\s*,\\s*\"order\"\\s*:\\s*\"rollout\"\\s*,\\s*\"stage\"\\s*:\\s*\"Completed\"\\s*,\\s*\"state\"\\s*:\\s*\"Ready\"\\s*,\\s*\"stages\"\\s*:\\s*\\[\\s*\"Review\"\\s*,\\s*\"Release\"\\s*\\]\\s*,\\s*\"review_prerequisites\"\\s*:\\s*\\[\\s*\"reviewer\"\\s*,\\s*\"tests\"\\s*\\]\\s*,\\s*\"evaluation\"\\s*:\\s*\"unknown\"\\s*,\\s*\"approval\"\\s*:\\s*\"unknown\"\\s*,\\s*\"runtime_health\"\\s*:\\s*\"unknown\"\\s*,\\s*\"absent_workflow_governance\"\\s*:\\s*\"unknown\"\\s*,\\s*\"denied_evaluation\"\\s*:\\s*\"unknown\"\\s*,\\s*\"runtime_server_version\"\\s*:\\s*\"unknown\"\\s*,\\s*\"provenance\"\\s*:\\s*\"authored\"\\s*,\\s*\"live_acceptance_proven\"\\s*:\\s*false\\s*\\}\\s*$"
target: last_message
flags: s
