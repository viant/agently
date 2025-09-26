# Safety Note

This file is a placeholder and does not affect the AI unless included in a request.  
Replace or remove it when defining real agent behavior.

# Example System Knowledge File

This is a placeholder file for the `system_knowledge/` folder.

It contains no active configuration or behavior rules.  
If included in the `system` message by mistake, it can be safely ignored.

---

## 📌 Purpose

Files in the `system_knowledge/` folder are used to define **default behavior, tone, formatting, and rules** for AI agents.

They are passed into the **`system` message** of an LLM request and should influence how the agent behaves **regardless of user input**.

This is the place to configure what the agent *is*, *how it should act*, and *what rules it must always follow*.

---

## ✅ Appropriate Content for This Folder

You may replace this file with real system-level knowledge such as:

### 🧠 Agent Identity or Role

- "You are a technical assistant that generates configuration files."
- "You help users write structured JSON based on input prompts."

### 🧾 Formatting & Output Rules

- "Always respond with valid YAML, without explanations."
- "Use snake_case for all keys."
- "Never include comments unless asked."

### 📜 Behavioral Constraints

- "Only include optional logic (e.g., pagination, joins, filters) if explicitly requested."
- "If a required field is missing, return a template with placeholders."

### 🔐 Safety Boundaries

- "Do not give legal or medical advice."
- "Avoid making assumptions when data is missing."

### ⚙️ Enforcement Rules

If your agent always follows strict generation logic (e.g., for file generation, code templates, schema validation), this folder is the right place for those rules. For example:

- "Each generated file must include a standard header block."
- "Alias SQL tables using their lowercase names."
- "Validate that all referenced columns exist before generating output."

---

## ⚠️ What Not to Include Here

System knowledge should not include instructions that:

- Are specific to **only one request**
- Refer to **specific datasets, examples, or tasks**
- Depend on user-provided context

These belong in the `knowledge/` folder and go into the `user` message.

---

## ✅ Summary

| Use This Folder For...                      | Not For...                          |
|--------------------------------------------|-------------------------------------|
| Agent behavior and formatting rules        | Prompt-specific instructions        |
| Always-enforced output constraints         | One-time data schemas               |
| Safety, tone, and default assumptions       | Dynamic examples or user inputs     |
| Role and responsibility descriptions       | Table-specific logic or filters     |

---
