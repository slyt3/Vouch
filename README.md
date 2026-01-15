# Vouch: The Agent Black Box

**Vouch** is a "flight recorder" for AI agents. It captures every move your agent makes, signs it cryptographically, and stores it in a ledger that no one can change—not even the agent.

## Why Vouch?
When AI agents move money, delete files, or send emails, you need a Ledger of Truth.
*   **Accountability**: If an agent makes a mistake, you have the signed proof.
*   **Security**: Agents can't "delete their tracks" if they are compromised.
*   **Trust**: You can finally let agents run autonomously because Vouch won't let them do anything risky without your permission.

## Simple Design & Architecture
Vouch is designed to be Zero-Config and Zero-Code-Change:

1.  **The Proxy**: Vouch sits between your Agent and its Tools. It listens, logs, and forwards.
2.  **The Ledger**: A local SQLite database where every event is hashed and chained (like a blockchain, but simpler).
3.  **The Policy**: A simple YAML file where you say: "If the agent tries to delete anything, stop it and ask me first."

```text
 [ Your Agent ]  ---- (Tool Call) ----> [ Vouch Proxy ] ----> [ Actual Tool ]
                                             |
                                     [ Signed Ledger ]
```

## How to Use It
It's as simple as 1-2-3:

1.  **Define Rules**: List risky tools in `vouch-policy.yaml`.
2.  **Start Proxy**: Run `./vouch-proxy`. Point your agent to Vouch's address.
3.  **Verify**: Run `vouch-cli verify` at any time to prove the logs are authentic.

## Why this way?
We chose a Proxy Architecture because it's the only way to be 100% sure the agent isn't lying. By intercepting the actual network traffic, Vouch sees exactly what the agent is doing in the real world, regardless of what the agent says in its internal logs.
