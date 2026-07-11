// Package compiler extends Mady's memory system with strategy learning.
//
// It tracks which execution strategies succeed or fail for different goal types,
// and uses ε-greedy exploration to balance exploiting known-good strategies
// against exploring new ones. Over time, the compiler builds a knowledge base
// of effective approaches for patent/legal domain tasks.
//
// Integration is via agentcore.Extension + LifecycleProvider:
//   - BeforeTurn: select a strategy based on goal + history
//   - AfterTurn: record the outcome and update strategy statistics
package compiler
