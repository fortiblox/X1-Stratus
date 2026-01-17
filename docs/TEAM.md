# X1-Stratus Team Requirements

## Overview

Building a production-grade blockchain verification node requires deep expertise across multiple domains. This document outlines the team structure needed to deliver X1-Stratus.

---

## Core Team (Required)

### Lead Protocol Engineer
**Count:** 1
**Compensation:** $180-250k/year or equivalent

**Responsibilities:**
- Overall technical architecture
- SVM implementation oversight
- Protocol conformance decisions
- Code review and quality standards

**Requirements:**
- 5+ years systems programming (Rust, Go, C++)
- Deep understanding of Solana/X1 protocol internals
- Experience with virtual machines or interpreters
- Blockchain consensus mechanisms knowledge
- Open source project leadership experience

**Nice to have:**
- Contributions to Solana/Agave/Firedancer
- Published research in distributed systems
- Experience with formal verification

---

### Senior Go Engineers
**Count:** 2-3
**Compensation:** $150-200k/year or equivalent

**Responsibilities:**
- Core component implementation (SVM, AccountsDB, Blockstore)
- Performance optimization
- Testing and conformance
- Documentation

**Requirements:**
- 4+ years Go development
- Experience with high-performance systems
- Understanding of database internals
- Familiarity with blockchain concepts
- Strong testing discipline

**Nice to have:**
- Experience with sBPF/eBPF
- Contributions to go-ethereum, cosmos-sdk, or similar
- Systems programming background (C/Rust)

---

### Blockchain Protocol Specialist
**Count:** 1
**Compensation:** $140-180k/year or equivalent

**Responsibilities:**
- STF rules implementation
- Consensus verification logic
- Epoch boundary handling
- Protocol upgrade compatibility

**Requirements:**
- 3+ years blockchain development
- Deep Solana protocol knowledge
- Experience with consensus mechanisms
- Understanding of cryptographic primitives
- Ability to read and understand Rust (Agave/Tachyon reference)

**Nice to have:**
- Validator operations experience
- Solana core contributions
- Academic background in distributed systems

---

## Extended Team (Recommended)

### DevOps / Infrastructure Engineer
**Count:** 1
**Compensation:** $120-160k/year or equivalent

**Responsibilities:**
- CI/CD pipeline
- Release management
- Docker/container builds
- Performance benchmarking infrastructure
- Monitoring and alerting

**Requirements:**
- 3+ years DevOps experience
- Go/Rust build systems
- Docker, Kubernetes
- Prometheus, Grafana
- Linux systems administration

---

### Technical Writer
**Count:** 1 (part-time or contract)
**Compensation:** $80-120k/year or equivalent

**Responsibilities:**
- API documentation
- Operator guides
- Architecture documentation
- Blog posts and updates

**Requirements:**
- Technical writing experience
- Ability to understand and explain complex systems
- Familiarity with blockchain concepts
- Markdown, documentation tools

---

### QA / Test Engineer
**Count:** 1
**Compensation:** $100-140k/year or equivalent

**Responsibilities:**
- Conformance test suite maintenance
- Integration testing
- Fuzzing infrastructure
- Regression testing

**Requirements:**
- 3+ years testing experience
- Go testing frameworks
- Property-based testing
- Fuzzing tools (go-fuzz, libfuzzer)

---

## Team Structure

```
                    ┌─────────────────┐
                    │  Project Lead   │
                    │  (Jack Levin)   │
                    └────────┬────────┘
                             │
              ┌──────────────┴──────────────┐
              │                             │
    ┌─────────▼─────────┐         ┌─────────▼─────────┐
    │  Lead Protocol    │         │     DevOps /      │
    │    Engineer       │         │  Infrastructure   │
    └─────────┬─────────┘         └───────────────────┘
              │
    ┌─────────┴─────────────────────┐
    │                               │
┌───▼───────────┐  ┌────────────────▼───────────────┐
│ Go Engineers  │  │  Protocol    │  QA / Testing  │
│   (2-3)       │  │  Specialist  │                │
└───────────────┘  └────────────────────────────────┘
```

---

## Budget Estimate

### Minimal Team (12-18 months to v1.0)
| Role | Count | Annual Cost |
|------|-------|-------------|
| Lead Protocol Engineer | 1 | $220k |
| Senior Go Engineers | 2 | $350k |
| Protocol Specialist | 1 | $160k |
| **Total** | **4** | **$730k/year** |

### Recommended Team (9-12 months to v1.0)
| Role | Count | Annual Cost |
|------|-------|-------------|
| Lead Protocol Engineer | 1 | $220k |
| Senior Go Engineers | 3 | $525k |
| Protocol Specialist | 1 | $160k |
| DevOps Engineer | 1 | $140k |
| QA Engineer | 1 | $120k |
| Technical Writer (PT) | 0.5 | $50k |
| **Total** | **7.5** | **$1.2M/year** |

---

## Hiring Channels

### Where to Find Candidates

1. **Solana Ecosystem**
   - Solana Discord (#jobs, #developers)
   - Solana Foundation network
   - Superteam DAO
   - Mithril/Overclock team referrals

2. **Go Community**
   - Gophers Slack
   - GopherCon
   - go-ethereum contributor network
   - Cosmos ecosystem

3. **General Blockchain**
   - Crypto Jobs List
   - Web3 Career
   - AngelList
   - LinkedIn (blockchain keywords)

4. **Open Source**
   - GitHub sponsors/contributors
   - Gitcoin grants
   - Protocol Labs network

---

## Interview Process

### Stage 1: Technical Screen (1 hour)
- Background and motivation
- Systems programming concepts
- Blockchain fundamentals
- Go proficiency assessment

### Stage 2: Technical Deep Dive (2 hours)
- Architecture design exercise
- Code review exercise
- Protocol knowledge assessment
- Past project discussion

### Stage 3: Paid Trial (1-2 weeks)
- Small implementation task
- Code quality assessment
- Communication evaluation
- Team fit

### Stage 4: Offer
- Compensation negotiation
- Start date
- Equity/token discussion (if applicable)

---

## Remote Work Policy

X1-Stratus is a **remote-first** project.

- Async communication (Discord, GitHub)
- Weekly sync calls (flexible timezone)
- Quarterly in-person meetups (optional)
- Results-oriented, not hours-oriented

---

## Getting Started

Interested candidates should:

1. Review the [README](../README.md) and [ROADMAP](ROADMAP.md)
2. Explore [Mithril](https://github.com/Overclock-Validator/mithril) codebase
3. Join the X1 Discord
4. Submit resume + GitHub profile + cover letter

Contact: [TBD - add email/Discord]
