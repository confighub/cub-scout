# Agentic GitOps observability: what "for an agent" means

Adopted as a working principle by the maintainer on 2026-11-13. This page
defines the term, says where cub-scout sits, and gives the questions to ask of
every capability. The capability table at the end is a first pass: it says what
each capability would need, not what is built. What is planned is in the
[agent plan](../roadmap-3.0-execution.md#agent-plan--2026-11-13).

## The definition

In GitOps a person changed a file, a controller delivered it, and the person
watched a dashboard until it looked right. In agentic GitOps the agent makes
the change, and the agent, or a gate standing in for a person, has to decide
whether it worked. Nobody is watching.

**Agentic GitOps observability is the evidence that lets an agent close its
own change loop.** Before it writes: who owns this, and will my change hold.
After: did exactly this revision reach the running workload, is it still
converging, and what is there that nobody recorded. It comes from a party that
did not make the change, in a form a machine can check, at a cost the agent
can budget.

It is not metrics and logs (is the system healthy), and it is not a GitOps
dashboard (is the app synced). Those answer a person who interprets what they
see. This answers a program that acts on what it is told.

## What changes when the reader is an agent

| | For a person | For an agent |
|---|---|---|
| Question | "Is it healthy?" | "Did *my* revision land?" A healthy Pod on the old image is a wrong yes. |
| Who says so | The deploy tool's own status | Self-report is not observation; the witness must not be the deployer. |
| Unknowns | A person notices a blank panel | An empty list reads as "nothing wrong", so every answer needs observed, not-observable or stale, with the reason. |
| Time | Waits and looks again | Needs "still converging, look again in N seconds" as a state distinct from failed. |
| Cost | Attention | Tokens and turns: one question, one call, a known size. |
| Safety | Ignores odd text in an event | May obey it, so cluster text is quoted data, and the tool cannot write. |

Two more follow from having many writers (several agents, people with
`kubectl`, self-healing controllers): **attribution**, meaning which manager
set a field and whether it came by the declared path; and **proof that
outlives the session**, meaning a receipt or recorded fact that the next
agent, a gate or an auditor can check without redoing the work.

## The loop, and cub-scout's place

1. **Look before writing:** who owns the field, what it is now, whether a
   controller will revert a direct change. cub-scout.
2. **Write,** through Git or ConfigHub. Never cub-scout.
3. **Deliver:** Flux, Argo CD, Sveltos, the sync bot. Not cub-scout.
4. **Witness:** did this revision land, is it converging, does the deployer's
   own report match the cluster. cub-scout.
5. **Record** a receipt or fact tied to the revision. cub-scout writes the
   evidence; ConfigHub keeps it.
6. **Judge:** promote, wait, roll back, ask a person. Pilot or a gate, not
   cub-scout.

## What makes it critical and not just nice to have

A dashboard is optional. A tool is critical when a write path depends on it.
There are four such places:

- **The agent's own skill:** "after any apply, call the landing check before
  reporting success".
- **The look-first read:** the same plugins call it before a write.
- **The judge's evidence:** Pilot already names cub-scout as its provider.
- **A promotion gate.** ConfigHub Attestations can be required by a
  ChangeWorkflow. A delivery attestation ("revision R was observed running in
  cluster C at time T, by a witness that is not the deployer") is the natural
  prerequisite for promoting R. cub-scout builds receipts and reads
  Attestations today; it does not yet produce one a workflow can require, and
  that needs ConfigHub's agreement.

Success is counted in the skills, allowlists and workflows that name
cub-scout.

## The principle

For every capability, old or new, ask what it means for an agent, with the six
rows above as the questions:

1. **Question.** Which question of an agent's does this answer, and is the
   answer anchored to a revision, digest or object identity?
2. **Who says so.** Whose word does each fact rest on? Is any of it the
   deployer's or the writer's own report, and does the answer say so?
3. **Unknowns.** Can an agent tell "nothing found" from "could not look" and
   from "out of date"?
4. **Time.** Can it tell "not yet" from "no", and does it know when to look
   again?
5. **Cost.** Is it one call, of a declared size?
6. **Safety.** Is text from the cluster marked as quoted data, and is the
   capability read-only?

A capability that has no good answer for an agent is not removed for that
reason: people use cub-scout too, and the CLI and TUI stay equal renderings of
one model. The answer is written down, and where it adds nothing for an agent,
we say so.

## First pass over today's capabilities

Not verified command by command. "Missing" means what an agent would need
that the capability is not known to give today.

| Capability | For a person | For an agent | Missing for an agent |
|---|---|---|---|
| `map` | Browse who owns what | "List what owner X manages" or "what is unmanaged in namespace N", as a bounded answer | A declared size, and paging in place of a full inventory |
| `trace` | Follow one object back to its source | "Which revision is this object from, and by what evidence?" | Whose word each link rests on |
| `explain` | Read why an object is as it is | "Who set this field, and will a controller revert my change?" | The revert prediction as a field; convergence state |
| `doctor`, `scan` | See what needs attention | "What should I read next?" as exact commands | Quoted cluster text; a size ceiling |
| `gitops status` | See what controllers report | "Is delivery settled, converging or failed, per controller?" | A recheck time; the witness rule on every reported status |
| `release_check`, `compare source-truth` | Check a release against the cluster | "Did my revision land?" | `converging` as a state, and a bounded wait |
| `compare three-way` | See drift between intended, recorded and live | "What changed that no one recorded?" | The unrecorded set as its own answer |
| `receipt` | Keep a record for review | A fact the next agent or a gate can check without redoing the work | A form a ChangeWorkflow can require; a signature |
| `import` preview | Plan an adoption | "What would be created, and what is ambiguous?" before an agent proposes it | Not assessed |
| TUI | Explore interactively | Nothing: an agent does not use it. It must show the same model as the CLI and MCP | Nothing |
