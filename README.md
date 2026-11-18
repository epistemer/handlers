# handlers

neozp's route package. Planned; nothing here works yet.

- **Admin routes** are declared by comment directives above handler methods
  and checked like code: a broken directive, a duplicate key or a parameter
  that doesn't match its wildcard is an error with a position.
- **API routes** come from one OpenAPI file.
- Both become one route table, from which genx generators write the
  registration on `net/http`, typed URL builders, i18n labels
  (`route.<key>`) and [auth](https://github.com/epistemer/auth) permissions.
- A language server, a VS Code extension, a Claude Code plugin and a Gemini
  CLI extension report directive faults while you, or an agent, write them.

The design is [docs/brief.md](docs/brief.md) and the plan
[docs/roadmap.md](docs/roadmap.md).

MIT licensed.
