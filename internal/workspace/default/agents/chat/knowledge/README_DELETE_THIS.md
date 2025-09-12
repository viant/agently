# 📘 README — Placeholder Knowledge File (Delete This)

This file is a **neutral placeholder** and **does not contain any useful knowledge** for the AI agent.  
If included in a prompt by mistake, it can be safely ignored.

---

## 📂 Purpose of the `knowledge/` Folder

This folder is intended to hold **task-specific or contextual knowledge** that will be passed into the **`user` message** of an LLM request. These files are used to guide the AI agent’s decisions based on the current task — such as generating structured output, interpreting data, or transforming input formats.

This file is here to help you get started. You should **delete or replace it** when you add real knowledge content.

---

## ✅ You May Replace This File With:

- A JSON or YAML **schema** for structured outputs
- A list of **field definitions** and their descriptions
- A **naming convention guide** (e.g., snake_case, camelCase)
- A sample **output template** with example data
- A list of **accepted types**, **keywords**, or **tag mappings**
- A **validation rule set** for inputs or output formatting
- A reference for **API request/response shape**
- **Glossaries** or internal **term definitions**
- Reusable **boilerplate text** (e.g., for policy documents)
- Configuration specs (e.g., flags, features, toggles)

---

## ❌ This File Should NOT Be Used For:

- Describing how the agent behaves or responds (→ that goes in `system_knowledge/`)
- Instructions about tone, response format, or safety
- Rules that are meant to apply to *all tasks*
- General-purpose constraints or defaults

---

## 💡 Tip for Designing Knowledge Files

Ask yourself:
> “What domain-specific context would a person need to see in order to do this task well?”

Put that in the `knowledge/` folder — the AI will read it the same way.

---

## 🔐 Reminder

This file is safe to keep in your repo, but should **never be included in a real prompt**.  
It contains **no operational logic**, and is only meant as a template for humans.

---

🧹 **Delete this file when you’re ready to add real knowledge.**
