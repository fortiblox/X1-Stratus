# X1-Stratus Roadmap

## Overview

Stratus development is divided into three major milestones, following the proven path of Mithril's development for Solana. Each milestone builds upon the previous, culminating in a production-ready lightweight verification node.

---

## Milestone 1: SVM Implementation

**Goal:** Complete, working implementation of the X1 Virtual Machine in Go

**Duration:** 4-6 months

**Deliverables:**

### Core Execution Engine
- [ ] sBPF virtual machine / interpreter
- [ ] Instruction execution pipeline
- [ ] Compute unit metering
- [ ] Memory management

### Runtime
- [ ] Account loading and validation
- [ ] Cross-Program Invocation (CPI)
- [ ] Program deployment and upgrades
- [ ] Rent exemption handling

### Syscalls
- [ ] sol_invoke_signed
- [ ] sol_log / sol_log_64 / sol_log_pubkey
- [ ] sol_sha256 / sol_keccak256
- [ ] sol_secp256k1_recover
- [ ] sol_create_program_address
- [ ] sol_get_clock_sysvar (and other sysvars)
- [ ] sol_memcpy / sol_memmove / sol_memset / sol_memcmp

### Native Programs
- [ ] System Program
- [ ] Vote Program
- [ ] Stake Program
- [ ] BPF Loader Program
- [ ] Address Lookup Table Program
- [ ] Config Program

### Pre-compiles
- [ ] Ed25519 signature verification
- [ ] secp256k1 signature recovery

### Testing
- [ ] Integration with Firedancer conformance test suite
- [ ] Unit tests for all native programs
- [ ] Fuzzing infrastructure

**Success Criteria:** Execute individual X1 transactions with results matching Tachyon/Agave

---

## Milestone 2: Block Replay

**Goal:** Sync from network and replay confirmed blocks

**Duration:** 4-6 months

**Deliverables:**

### Snapshot Handling
- [ ] Snapshot download from trusted sources
- [ ] Snapshot format decoding (accounts, bank state)
- [ ] Incremental snapshot support
- [ ] Snapshot verification

### AccountsDB
- [ ] Account storage backend
- [ ] Account indexing
- [ ] State pruning
- [ ] Epoch accounts hash calculation

### Blockstore
- [ ] Block storage format
- [ ] Block retrieval by slot
- [ ] Shred reconstruction (optional)

### State Transition Function (STF)
- [ ] Block-level rules
  - [ ] Compute unit limits
  - [ ] Block size validation
  - [ ] Transaction ordering
  - [ ] Entry hash verification
  - [ ] Bank hash calculation
- [ ] Epoch-level rules
  - [ ] Leader schedule calculation
  - [ ] Stake weight updates
  - [ ] Reward distribution
  - [ ] Epoch accounts hash

### Block Fetching
- [ ] Geyser plugin integration
- [ ] RPC getBlock polling
- [ ] Custom block relay protocol (optional)

### RPC Interface
- [ ] getHealth
- [ ] getSlot
- [ ] getVersion
- [ ] getBalance
- [ ] getAccountInfo
- [ ] getTransaction
- [ ] getBlock
- [ ] sendTransaction (forward to validators)
- [ ] simulateTransaction

**Success Criteria:** Sync from snapshot and replay blocks with bank hashes matching network

---

## Milestone 3: Optimization & Production

**Goal:** Production-ready, optimized verification node

**Duration:** 3-4 months

**Deliverables:**

### Performance
- [ ] Batched block replay (Erigon-style staged sync)
- [ ] BlockSTM parallel transaction execution
- [ ] GPU signature verification (optional)
- [ ] Memory-mapped AccountsDB
- [ ] Efficient state pruning

### Reliability
- [ ] Graceful restart / crash recovery
- [ ] Automatic snapshot catchup
- [ ] Health monitoring and metrics
- [ ] Prometheus / Grafana integration

### Distribution
- [ ] Cross-platform builds (Linux, macOS, Windows)
- [ ] Docker images
- [ ] One-line installer
- [ ] Verifiable reproducible builds

### Documentation
- [ ] Operator guide
- [ ] Architecture documentation
- [ ] API reference
- [ ] Contribution guidelines

**Success Criteria:** Run on consumer hardware (4GB RAM, 4 cores) with <10s block lag

---

## Timeline

```
2026 Q1  ──────────────────────────────────────────────────────
         │ Project Setup
         │ Team Assembly
         │ M1: SVM Core Start
         │
2026 Q2  ──────────────────────────────────────────────────────
         │ M1: Native Programs
         │ M1: Syscalls
         │ M1: Conformance Testing
         │
2026 Q3  ──────────────────────────────────────────────────────
         │ M1: Complete
         │ M2: Snapshot & AccountsDB Start
         │ M2: Block Fetching
         │
2026 Q4  ──────────────────────────────────────────────────────
         │ M2: STF Rules
         │ M2: RPC Interface
         │ M2: Complete
         │
2027 Q1  ──────────────────────────────────────────────────────
         │ M3: Optimization
         │ M3: Production Hardening
         │ M3: Release v1.0
         │
```

---

## Dependencies

### External
- [Mithril](https://github.com/Overclock-Validator/mithril) — Reference implementation, potential code reuse
- [Radiance](https://github.com/gagliardetto/radiance) — Go SDK, sBPF VM foundation
- [Firedancer conformance suite](https://github.com/firedancer-io/firedancer) — Test vectors

### Internal
- X1 Tachyon source — Protocol reference
- X1 Mainnet — Live testing environment
- X1-Aether — Comparison baseline

---

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Protocol changes in Tachyon | High | Close collaboration with X1 core team |
| SVM edge cases | Medium | Extensive conformance testing |
| Performance targets not met | Medium | Iterative optimization, accept higher specs |
| Team availability | High | Competitive compensation, remote-first |

---

## Success Metrics

| Metric | Target |
|--------|--------|
| RAM usage | < 4 GB |
| CPU usage | < 4 cores |
| Block lag | < 10 seconds |
| Sync time (from snapshot) | < 1 hour |
| RPC latency | < 100ms |
| Uptime | > 99.9% |
