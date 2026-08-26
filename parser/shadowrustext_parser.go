// Code generated from grammar/ShadowRustExt.g4 by ANTLR 4.13.1. DO NOT EDIT.

package parser // ShadowRustExt
import (
	"fmt"
	"strconv"
	"sync"

	"github.com/antlr4-go/antlr/v4"
)

// Suppress unused import errors
var _ = fmt.Printf
var _ = strconv.Itoa
var _ = sync.Once{}

type ShadowRustExtParser struct {
	*antlr.BaseParser
}

var ShadowRustExtParserStaticData struct {
	once                   sync.Once
	serializedATN          []int32
	LiteralNames           []string
	SymbolicNames          []string
	RuleNames              []string
	PredictionContextCache *antlr.PredictionContextCache
	atn                    *antlr.ATN
	decisionToDFA          []*antlr.DFA
}

func shadowrustextParserInit() {
	staticData := &ShadowRustExtParserStaticData
	staticData.LiteralNames = []string{
		"", "'{'", "'}'", "'='", "';'", "'('", "')'", "','", "'>='", "'>'",
		"'<='", "'<'", "'=='", "'!='", "'+'", "'-'", "'*'", "'/'", "'container'",
		"'network'", "'resilience'", "'activate'", "'sentinels'", "'update_trait'",
		"'vote'", "'shard'", "'async_stagger'", "'count'", "'+='", "'-='", "'tx'",
		"'buy'", "'from'", "'to'", "'amount'", "'if'", "'mint'", "'epoch'",
		"'validate'", "'stage'", "'queue'", "'insert'", "'positions'", "'bank'",
		"'deposit'", "'atr'",
	}
	staticData.SymbolicNames = []string{
		"", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "",
		"", "CONTAINER", "NETWORK", "RESILIENCE", "ACTIVATE", "SENTINELS", "UPDATE_TRAIT",
		"VOTE", "SHARD", "ASYNC_STAGGER", "COUNT", "PLUSEQ", "MINUSEQ", "TX",
		"BUY", "FROM", "TO", "AMOUNT", "IF", "MINT", "EPOCH", "VALIDATE", "STAGE",
		"QUEUE", "INSERT", "POSITIONS", "BANK", "DEPOSIT", "ATR", "ID", "NUMBER",
		"WS", "COMMENT",
	}
	staticData.RuleNames = []string{
		"statement", "containerStatement", "containerField", "networkStatement",
		"networkField", "resilienceStatement", "activateStatement", "updateTraitStatement",
		"voteStatement", "shardStatement", "asyncStaggerStatement", "staggerField",
		"factor", "program", "ifStatement", "txStatement", "mintStatement",
		"validateStatement", "queueStatement", "bankStatement", "assignment",
		"condition", "expr", "relExpr", "arithExpr", "term",
	}
	staticData.PredictionContextCache = antlr.NewPredictionContextCache()
	staticData.serializedATN = []int32{
		4, 1, 49, 277, 2, 0, 7, 0, 2, 1, 7, 1, 2, 2, 7, 2, 2, 3, 7, 3, 2, 4, 7,
		4, 2, 5, 7, 5, 2, 6, 7, 6, 2, 7, 7, 7, 2, 8, 7, 8, 2, 9, 7, 9, 2, 10, 7,
		10, 2, 11, 7, 11, 2, 12, 7, 12, 2, 13, 7, 13, 2, 14, 7, 14, 2, 15, 7, 15,
		2, 16, 7, 16, 2, 17, 7, 17, 2, 18, 7, 18, 2, 19, 7, 19, 2, 20, 7, 20, 2,
		21, 7, 21, 2, 22, 7, 22, 2, 23, 7, 23, 2, 24, 7, 24, 2, 25, 7, 25, 1, 0,
		1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0,
		1, 0, 1, 0, 3, 0, 68, 8, 0, 1, 1, 1, 1, 1, 1, 5, 1, 73, 8, 1, 10, 1, 12,
		1, 76, 9, 1, 1, 1, 1, 1, 1, 2, 1, 2, 1, 2, 1, 2, 1, 2, 1, 3, 1, 3, 1, 3,
		5, 3, 88, 8, 3, 10, 3, 12, 3, 91, 9, 3, 1, 3, 1, 3, 1, 4, 1, 4, 1, 4, 1,
		4, 1, 4, 1, 5, 1, 5, 1, 5, 1, 5, 1, 5, 5, 5, 105, 8, 5, 10, 5, 12, 5, 108,
		9, 5, 1, 5, 1, 5, 1, 6, 1, 6, 1, 6, 1, 6, 1, 7, 1, 7, 1, 7, 1, 7, 1, 7,
		1, 7, 1, 7, 1, 8, 1, 8, 1, 8, 1, 8, 1, 8, 1, 9, 1, 9, 1, 9, 1, 9, 3, 9,
		132, 8, 9, 1, 9, 1, 9, 1, 10, 1, 10, 1, 10, 5, 10, 139, 8, 10, 10, 10,
		12, 10, 142, 9, 10, 1, 10, 1, 10, 1, 11, 1, 11, 1, 11, 1, 11, 1, 11, 1,
		12, 1, 12, 1, 12, 1, 12, 1, 12, 1, 12, 1, 12, 3, 12, 158, 8, 12, 1, 13,
		5, 13, 161, 8, 13, 10, 13, 12, 13, 164, 9, 13, 1, 13, 1, 13, 1, 14, 1,
		14, 1, 14, 1, 14, 5, 14, 172, 8, 14, 10, 14, 12, 14, 175, 9, 14, 1, 14,
		1, 14, 1, 15, 1, 15, 1, 15, 1, 15, 1, 15, 1, 15, 1, 15, 1, 15, 1, 15, 1,
		15, 1, 15, 5, 15, 190, 8, 15, 10, 15, 12, 15, 193, 9, 15, 1, 15, 1, 15,
		1, 16, 1, 16, 1, 16, 1, 16, 1, 16, 1, 16, 3, 16, 203, 8, 16, 1, 16, 1,
		16, 1, 17, 1, 17, 1, 17, 1, 17, 1, 17, 1, 17, 5, 17, 213, 8, 17, 10, 17,
		12, 17, 216, 9, 17, 1, 17, 1, 17, 1, 18, 1, 18, 1, 18, 1, 18, 1, 18, 1,
		18, 1, 18, 5, 18, 227, 8, 18, 10, 18, 12, 18, 230, 9, 18, 1, 18, 1, 18,
		1, 19, 1, 19, 1, 19, 1, 19, 1, 19, 1, 19, 1, 19, 1, 20, 1, 20, 1, 20, 1,
		20, 1, 20, 1, 21, 1, 21, 1, 22, 1, 22, 1, 22, 3, 22, 251, 8, 22, 1, 23,
		1, 23, 1, 23, 5, 23, 256, 8, 23, 10, 23, 12, 23, 259, 9, 23, 1, 24, 1,
		24, 1, 24, 5, 24, 264, 8, 24, 10, 24, 12, 24, 267, 9, 24, 1, 25, 1, 25,
		1, 25, 5, 25, 272, 8, 25, 10, 25, 12, 25, 275, 9, 25, 1, 25, 0, 0, 26,
		0, 2, 4, 6, 8, 10, 12, 14, 16, 18, 20, 22, 24, 26, 28, 30, 32, 34, 36,
		38, 40, 42, 44, 46, 48, 50, 0, 4, 2, 0, 3, 3, 28, 29, 1, 0, 8, 13, 1, 0,
		14, 15, 1, 0, 16, 17, 282, 0, 67, 1, 0, 0, 0, 2, 69, 1, 0, 0, 0, 4, 79,
		1, 0, 0, 0, 6, 84, 1, 0, 0, 0, 8, 94, 1, 0, 0, 0, 10, 99, 1, 0, 0, 0, 12,
		111, 1, 0, 0, 0, 14, 115, 1, 0, 0, 0, 16, 122, 1, 0, 0, 0, 18, 127, 1,
		0, 0, 0, 20, 135, 1, 0, 0, 0, 22, 145, 1, 0, 0, 0, 24, 157, 1, 0, 0, 0,
		26, 162, 1, 0, 0, 0, 28, 167, 1, 0, 0, 0, 30, 178, 1, 0, 0, 0, 32, 196,
		1, 0, 0, 0, 34, 206, 1, 0, 0, 0, 36, 219, 1, 0, 0, 0, 38, 233, 1, 0, 0,
		0, 40, 240, 1, 0, 0, 0, 42, 245, 1, 0, 0, 0, 44, 247, 1, 0, 0, 0, 46, 252,
		1, 0, 0, 0, 48, 260, 1, 0, 0, 0, 50, 268, 1, 0, 0, 0, 52, 68, 3, 28, 14,
		0, 53, 68, 3, 30, 15, 0, 54, 68, 3, 32, 16, 0, 55, 68, 3, 34, 17, 0, 56,
		68, 3, 36, 18, 0, 57, 68, 3, 38, 19, 0, 58, 68, 3, 2, 1, 0, 59, 68, 3,
		6, 3, 0, 60, 68, 3, 10, 5, 0, 61, 68, 3, 12, 6, 0, 62, 68, 3, 14, 7, 0,
		63, 68, 3, 16, 8, 0, 64, 68, 3, 18, 9, 0, 65, 68, 3, 20, 10, 0, 66, 68,
		3, 40, 20, 0, 67, 52, 1, 0, 0, 0, 67, 53, 1, 0, 0, 0, 67, 54, 1, 0, 0,
		0, 67, 55, 1, 0, 0, 0, 67, 56, 1, 0, 0, 0, 67, 57, 1, 0, 0, 0, 67, 58,
		1, 0, 0, 0, 67, 59, 1, 0, 0, 0, 67, 60, 1, 0, 0, 0, 67, 61, 1, 0, 0, 0,
		67, 62, 1, 0, 0, 0, 67, 63, 1, 0, 0, 0, 67, 64, 1, 0, 0, 0, 67, 65, 1,
		0, 0, 0, 67, 66, 1, 0, 0, 0, 68, 1, 1, 0, 0, 0, 69, 70, 5, 18, 0, 0, 70,
		74, 5, 1, 0, 0, 71, 73, 3, 4, 2, 0, 72, 71, 1, 0, 0, 0, 73, 76, 1, 0, 0,
		0, 74, 72, 1, 0, 0, 0, 74, 75, 1, 0, 0, 0, 75, 77, 1, 0, 0, 0, 76, 74,
		1, 0, 0, 0, 77, 78, 5, 2, 0, 0, 78, 3, 1, 0, 0, 0, 79, 80, 5, 46, 0, 0,
		80, 81, 5, 3, 0, 0, 81, 82, 3, 44, 22, 0, 82, 83, 5, 4, 0, 0, 83, 5, 1,
		0, 0, 0, 84, 85, 5, 19, 0, 0, 85, 89, 5, 1, 0, 0, 86, 88, 3, 8, 4, 0, 87,
		86, 1, 0, 0, 0, 88, 91, 1, 0, 0, 0, 89, 87, 1, 0, 0, 0, 89, 90, 1, 0, 0,
		0, 90, 92, 1, 0, 0, 0, 91, 89, 1, 0, 0, 0, 92, 93, 5, 2, 0, 0, 93, 7, 1,
		0, 0, 0, 94, 95, 5, 46, 0, 0, 95, 96, 5, 3, 0, 0, 96, 97, 3, 44, 22, 0,
		97, 98, 5, 4, 0, 0, 98, 9, 1, 0, 0, 0, 99, 100, 5, 20, 0, 0, 100, 101,
		5, 35, 0, 0, 101, 102, 3, 42, 21, 0, 102, 106, 5, 1, 0, 0, 103, 105, 3,
		0, 0, 0, 104, 103, 1, 0, 0, 0, 105, 108, 1, 0, 0, 0, 106, 104, 1, 0, 0,
		0, 106, 107, 1, 0, 0, 0, 107, 109, 1, 0, 0, 0, 108, 106, 1, 0, 0, 0, 109,
		110, 5, 2, 0, 0, 110, 11, 1, 0, 0, 0, 111, 112, 5, 21, 0, 0, 112, 113,
		5, 22, 0, 0, 113, 114, 5, 4, 0, 0, 114, 13, 1, 0, 0, 0, 115, 116, 5, 23,
		0, 0, 116, 117, 5, 46, 0, 0, 117, 118, 5, 46, 0, 0, 118, 119, 7, 0, 0,
		0, 119, 120, 3, 44, 22, 0, 120, 121, 5, 4, 0, 0, 121, 15, 1, 0, 0, 0, 122,
		123, 5, 24, 0, 0, 123, 124, 5, 46, 0, 0, 124, 125, 5, 46, 0, 0, 125, 126,
		5, 4, 0, 0, 126, 17, 1, 0, 0, 0, 127, 128, 5, 25, 0, 0, 128, 131, 5, 46,
		0, 0, 129, 130, 5, 27, 0, 0, 130, 132, 3, 44, 22, 0, 131, 129, 1, 0, 0,
		0, 131, 132, 1, 0, 0, 0, 132, 133, 1, 0, 0, 0, 133, 134, 5, 4, 0, 0, 134,
		19, 1, 0, 0, 0, 135, 136, 5, 26, 0, 0, 136, 140, 5, 1, 0, 0, 137, 139,
		3, 22, 11, 0, 138, 137, 1, 0, 0, 0, 139, 142, 1, 0, 0, 0, 140, 138, 1,
		0, 0, 0, 140, 141, 1, 0, 0, 0, 141, 143, 1, 0, 0, 0, 142, 140, 1, 0, 0,
		0, 143, 144, 5, 2, 0, 0, 144, 21, 1, 0, 0, 0, 145, 146, 5, 46, 0, 0, 146,
		147, 5, 3, 0, 0, 147, 148, 3, 44, 22, 0, 148, 149, 5, 4, 0, 0, 149, 23,
		1, 0, 0, 0, 150, 158, 5, 46, 0, 0, 151, 158, 5, 34, 0, 0, 152, 158, 5,
		47, 0, 0, 153, 154, 5, 5, 0, 0, 154, 155, 3, 44, 22, 0, 155, 156, 5, 6,
		0, 0, 156, 158, 1, 0, 0, 0, 157, 150, 1, 0, 0, 0, 157, 151, 1, 0, 0, 0,
		157, 152, 1, 0, 0, 0, 157, 153, 1, 0, 0, 0, 158, 25, 1, 0, 0, 0, 159, 161,
		3, 0, 0, 0, 160, 159, 1, 0, 0, 0, 161, 164, 1, 0, 0, 0, 162, 160, 1, 0,
		0, 0, 162, 163, 1, 0, 0, 0, 163, 165, 1, 0, 0, 0, 164, 162, 1, 0, 0, 0,
		165, 166, 5, 0, 0, 1, 166, 27, 1, 0, 0, 0, 167, 168, 5, 35, 0, 0, 168,
		169, 3, 42, 21, 0, 169, 173, 5, 1, 0, 0, 170, 172, 3, 0, 0, 0, 171, 170,
		1, 0, 0, 0, 172, 175, 1, 0, 0, 0, 173, 171, 1, 0, 0, 0, 173, 174, 1, 0,
		0, 0, 174, 176, 1, 0, 0, 0, 175, 173, 1, 0, 0, 0, 176, 177, 5, 2, 0, 0,
		177, 29, 1, 0, 0, 0, 178, 179, 5, 30, 0, 0, 179, 180, 5, 31, 0, 0, 180,
		181, 5, 46, 0, 0, 181, 182, 5, 32, 0, 0, 182, 183, 5, 46, 0, 0, 183, 184,
		5, 33, 0, 0, 184, 185, 5, 46, 0, 0, 185, 186, 5, 34, 0, 0, 186, 187, 3,
		44, 22, 0, 187, 191, 5, 1, 0, 0, 188, 190, 3, 0, 0, 0, 189, 188, 1, 0,
		0, 0, 190, 193, 1, 0, 0, 0, 191, 189, 1, 0, 0, 0, 191, 192, 1, 0, 0, 0,
		192, 194, 1, 0, 0, 0, 193, 191, 1, 0, 0, 0, 194, 195, 5, 2, 0, 0, 195,
		31, 1, 0, 0, 0, 196, 197, 5, 36, 0, 0, 197, 198, 5, 46, 0, 0, 198, 199,
		5, 34, 0, 0, 199, 202, 3, 44, 22, 0, 200, 201, 5, 37, 0, 0, 201, 203, 3,
		44, 22, 0, 202, 200, 1, 0, 0, 0, 202, 203, 1, 0, 0, 0, 203, 204, 1, 0,
		0, 0, 204, 205, 5, 4, 0, 0, 205, 33, 1, 0, 0, 0, 206, 207, 5, 38, 0, 0,
		207, 208, 5, 46, 0, 0, 208, 209, 5, 39, 0, 0, 209, 210, 5, 47, 0, 0, 210,
		214, 5, 1, 0, 0, 211, 213, 3, 0, 0, 0, 212, 211, 1, 0, 0, 0, 213, 216,
		1, 0, 0, 0, 214, 212, 1, 0, 0, 0, 214, 215, 1, 0, 0, 0, 215, 217, 1, 0,
		0, 0, 216, 214, 1, 0, 0, 0, 217, 218, 5, 2, 0, 0, 218, 35, 1, 0, 0, 0,
		219, 220, 5, 40, 0, 0, 220, 221, 5, 41, 0, 0, 221, 222, 5, 46, 0, 0, 222,
		223, 5, 42, 0, 0, 223, 228, 3, 44, 22, 0, 224, 225, 5, 7, 0, 0, 225, 227,
		3, 44, 22, 0, 226, 224, 1, 0, 0, 0, 227, 230, 1, 0, 0, 0, 228, 226, 1,
		0, 0, 0, 228, 229, 1, 0, 0, 0, 229, 231, 1, 0, 0, 0, 230, 228, 1, 0, 0,
		0, 231, 232, 5, 4, 0, 0, 232, 37, 1, 0, 0, 0, 233, 234, 5, 43, 0, 0, 234,
		235, 5, 44, 0, 0, 235, 236, 5, 46, 0, 0, 236, 237, 5, 45, 0, 0, 237, 238,
		3, 44, 22, 0, 238, 239, 5, 4, 0, 0, 239, 39, 1, 0, 0, 0, 240, 241, 5, 46,
		0, 0, 241, 242, 5, 3, 0, 0, 242, 243, 3, 44, 22, 0, 243, 244, 5, 4, 0,
		0, 244, 41, 1, 0, 0, 0, 245, 246, 3, 44, 22, 0, 246, 43, 1, 0, 0, 0, 247,
		250, 3, 46, 23, 0, 248, 249, 5, 33, 0, 0, 249, 251, 5, 46, 0, 0, 250, 248,
		1, 0, 0, 0, 250, 251, 1, 0, 0, 0, 251, 45, 1, 0, 0, 0, 252, 257, 3, 48,
		24, 0, 253, 254, 7, 1, 0, 0, 254, 256, 3, 48, 24, 0, 255, 253, 1, 0, 0,
		0, 256, 259, 1, 0, 0, 0, 257, 255, 1, 0, 0, 0, 257, 258, 1, 0, 0, 0, 258,
		47, 1, 0, 0, 0, 259, 257, 1, 0, 0, 0, 260, 265, 3, 50, 25, 0, 261, 262,
		7, 2, 0, 0, 262, 264, 3, 50, 25, 0, 263, 261, 1, 0, 0, 0, 264, 267, 1,
		0, 0, 0, 265, 263, 1, 0, 0, 0, 265, 266, 1, 0, 0, 0, 266, 49, 1, 0, 0,
		0, 267, 265, 1, 0, 0, 0, 268, 273, 3, 24, 12, 0, 269, 270, 7, 3, 0, 0,
		270, 272, 3, 24, 12, 0, 271, 269, 1, 0, 0, 0, 272, 275, 1, 0, 0, 0, 273,
		271, 1, 0, 0, 0, 273, 274, 1, 0, 0, 0, 274, 51, 1, 0, 0, 0, 275, 273, 1,
		0, 0, 0, 17, 67, 74, 89, 106, 131, 140, 157, 162, 173, 191, 202, 214, 228,
		250, 257, 265, 273,
	}
	deserializer := antlr.NewATNDeserializer(nil)
	staticData.atn = deserializer.Deserialize(staticData.serializedATN)
	atn := staticData.atn
	staticData.decisionToDFA = make([]*antlr.DFA, len(atn.DecisionToState))
	decisionToDFA := staticData.decisionToDFA
	for index, state := range atn.DecisionToState {
		decisionToDFA[index] = antlr.NewDFA(state, index)
	}
}

// ShadowRustExtParserInit initializes any static state used to implement ShadowRustExtParser. By default the
// static state used to implement the parser is lazily initialized during the first call to
// NewShadowRustExtParser(). You can call this function if you wish to initialize the static state ahead
// of time.
func ShadowRustExtParserInit() {
	staticData := &ShadowRustExtParserStaticData
	staticData.once.Do(shadowrustextParserInit)
}

// NewShadowRustExtParser produces a new parser instance for the optional input antlr.TokenStream.
func NewShadowRustExtParser(input antlr.TokenStream) *ShadowRustExtParser {
	ShadowRustExtParserInit()
	this := new(ShadowRustExtParser)
	this.BaseParser = antlr.NewBaseParser(input)
	staticData := &ShadowRustExtParserStaticData
	this.Interpreter = antlr.NewParserATNSimulator(this, staticData.atn, staticData.decisionToDFA, staticData.PredictionContextCache)
	this.RuleNames = staticData.RuleNames
	this.LiteralNames = staticData.LiteralNames
	this.SymbolicNames = staticData.SymbolicNames
	this.GrammarFileName = "ShadowRustExt.g4"

	return this
}

// ShadowRustExtParser tokens.
const (
	ShadowRustExtParserEOF           = antlr.TokenEOF
	ShadowRustExtParserT__0          = 1
	ShadowRustExtParserT__1          = 2
	ShadowRustExtParserT__2          = 3
	ShadowRustExtParserT__3          = 4
	ShadowRustExtParserT__4          = 5
	ShadowRustExtParserT__5          = 6
	ShadowRustExtParserT__6          = 7
	ShadowRustExtParserT__7          = 8
	ShadowRustExtParserT__8          = 9
	ShadowRustExtParserT__9          = 10
	ShadowRustExtParserT__10         = 11
	ShadowRustExtParserT__11         = 12
	ShadowRustExtParserT__12         = 13
	ShadowRustExtParserT__13         = 14
	ShadowRustExtParserT__14         = 15
	ShadowRustExtParserT__15         = 16
	ShadowRustExtParserT__16         = 17
	ShadowRustExtParserCONTAINER     = 18
	ShadowRustExtParserNETWORK       = 19
	ShadowRustExtParserRESILIENCE    = 20
	ShadowRustExtParserACTIVATE      = 21
	ShadowRustExtParserSENTINELS     = 22
	ShadowRustExtParserUPDATE_TRAIT  = 23
	ShadowRustExtParserVOTE          = 24
	ShadowRustExtParserSHARD         = 25
	ShadowRustExtParserASYNC_STAGGER = 26
	ShadowRustExtParserCOUNT         = 27
	ShadowRustExtParserPLUSEQ        = 28
	ShadowRustExtParserMINUSEQ       = 29
	ShadowRustExtParserTX            = 30
	ShadowRustExtParserBUY           = 31
	ShadowRustExtParserFROM          = 32
	ShadowRustExtParserTO            = 33
	ShadowRustExtParserAMOUNT        = 34
	ShadowRustExtParserIF            = 35
	ShadowRustExtParserMINT          = 36
	ShadowRustExtParserEPOCH         = 37
	ShadowRustExtParserVALIDATE      = 38
	ShadowRustExtParserSTAGE         = 39
	ShadowRustExtParserQUEUE         = 40
	ShadowRustExtParserINSERT        = 41
	ShadowRustExtParserPOSITIONS     = 42
	ShadowRustExtParserBANK          = 43
	ShadowRustExtParserDEPOSIT       = 44
	ShadowRustExtParserATR           = 45
	ShadowRustExtParserID            = 46
	ShadowRustExtParserNUMBER        = 47
	ShadowRustExtParserWS            = 48
	ShadowRustExtParserCOMMENT       = 49
)

// ShadowRustExtParser rules.
const (
	ShadowRustExtParserRULE_statement             = 0
	ShadowRustExtParserRULE_containerStatement    = 1
	ShadowRustExtParserRULE_containerField        = 2
	ShadowRustExtParserRULE_networkStatement      = 3
	ShadowRustExtParserRULE_networkField          = 4
	ShadowRustExtParserRULE_resilienceStatement   = 5
	ShadowRustExtParserRULE_activateStatement     = 6
	ShadowRustExtParserRULE_updateTraitStatement  = 7
	ShadowRustExtParserRULE_voteStatement         = 8
	ShadowRustExtParserRULE_shardStatement        = 9
	ShadowRustExtParserRULE_asyncStaggerStatement = 10
	ShadowRustExtParserRULE_staggerField          = 11
	ShadowRustExtParserRULE_factor                = 12
	ShadowRustExtParserRULE_program               = 13
	ShadowRustExtParserRULE_ifStatement           = 14
	ShadowRustExtParserRULE_txStatement           = 15
	ShadowRustExtParserRULE_mintStatement         = 16
	ShadowRustExtParserRULE_validateStatement     = 17
	ShadowRustExtParserRULE_queueStatement        = 18
	ShadowRustExtParserRULE_bankStatement         = 19
	ShadowRustExtParserRULE_assignment            = 20
	ShadowRustExtParserRULE_condition             = 21
	ShadowRustExtParserRULE_expr                  = 22
	ShadowRustExtParserRULE_relExpr               = 23
	ShadowRustExtParserRULE_arithExpr             = 24
	ShadowRustExtParserRULE_term                  = 25
)

// IStatementContext is an interface to support dynamic dispatch.
type IStatementContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	IfStatement() IIfStatementContext
	TxStatement() ITxStatementContext
	MintStatement() IMintStatementContext
	ValidateStatement() IValidateStatementContext
	QueueStatement() IQueueStatementContext
	BankStatement() IBankStatementContext
	ContainerStatement() IContainerStatementContext
	NetworkStatement() INetworkStatementContext
	ResilienceStatement() IResilienceStatementContext
	ActivateStatement() IActivateStatementContext
	UpdateTraitStatement() IUpdateTraitStatementContext
	VoteStatement() IVoteStatementContext
	ShardStatement() IShardStatementContext
	AsyncStaggerStatement() IAsyncStaggerStatementContext
	Assignment() IAssignmentContext

	// IsStatementContext differentiates from other interfaces.
	IsStatementContext()
}

type StatementContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyStatementContext() *StatementContext {
	var p = new(StatementContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_statement
	return p
}

func InitEmptyStatementContext(p *StatementContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_statement
}

func (*StatementContext) IsStatementContext() {}

func NewStatementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *StatementContext {
	var p = new(StatementContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ShadowRustExtParserRULE_statement

	return p
}

func (s *StatementContext) GetParser() antlr.Parser { return s.parser }

func (s *StatementContext) IfStatement() IIfStatementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IIfStatementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IIfStatementContext)
}

func (s *StatementContext) TxStatement() ITxStatementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ITxStatementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(ITxStatementContext)
}

func (s *StatementContext) MintStatement() IMintStatementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IMintStatementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IMintStatementContext)
}

func (s *StatementContext) ValidateStatement() IValidateStatementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IValidateStatementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IValidateStatementContext)
}

func (s *StatementContext) QueueStatement() IQueueStatementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IQueueStatementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IQueueStatementContext)
}

func (s *StatementContext) BankStatement() IBankStatementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IBankStatementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IBankStatementContext)
}

func (s *StatementContext) ContainerStatement() IContainerStatementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IContainerStatementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IContainerStatementContext)
}

func (s *StatementContext) NetworkStatement() INetworkStatementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(INetworkStatementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(INetworkStatementContext)
}

func (s *StatementContext) ResilienceStatement() IResilienceStatementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IResilienceStatementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IResilienceStatementContext)
}

func (s *StatementContext) ActivateStatement() IActivateStatementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IActivateStatementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IActivateStatementContext)
}

func (s *StatementContext) UpdateTraitStatement() IUpdateTraitStatementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IUpdateTraitStatementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IUpdateTraitStatementContext)
}

func (s *StatementContext) VoteStatement() IVoteStatementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IVoteStatementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IVoteStatementContext)
}

func (s *StatementContext) ShardStatement() IShardStatementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IShardStatementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IShardStatementContext)
}

func (s *StatementContext) AsyncStaggerStatement() IAsyncStaggerStatementContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IAsyncStaggerStatementContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IAsyncStaggerStatementContext)
}

func (s *StatementContext) Assignment() IAssignmentContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IAssignmentContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IAssignmentContext)
}

func (s *StatementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *StatementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *StatementContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.EnterStatement(s)
	}
}

func (s *StatementContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.ExitStatement(s)
	}
}

func (s *StatementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ShadowRustExtVisitor:
		return t.VisitStatement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ShadowRustExtParser) Statement() (localctx IStatementContext) {
	localctx = NewStatementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 0, ShadowRustExtParserRULE_statement)
	p.SetState(67)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}

	switch p.GetTokenStream().LA(1) {
	case ShadowRustExtParserIF:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(52)
			p.IfStatement()
		}

	case ShadowRustExtParserTX:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(53)
			p.TxStatement()
		}

	case ShadowRustExtParserMINT:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(54)
			p.MintStatement()
		}

	case ShadowRustExtParserVALIDATE:
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(55)
			p.ValidateStatement()
		}

	case ShadowRustExtParserQUEUE:
		p.EnterOuterAlt(localctx, 5)
		{
			p.SetState(56)
			p.QueueStatement()
		}

	case ShadowRustExtParserBANK:
		p.EnterOuterAlt(localctx, 6)
		{
			p.SetState(57)
			p.BankStatement()
		}

	case ShadowRustExtParserCONTAINER:
		p.EnterOuterAlt(localctx, 7)
		{
			p.SetState(58)
			p.ContainerStatement()
		}

	case ShadowRustExtParserNETWORK:
		p.EnterOuterAlt(localctx, 8)
		{
			p.SetState(59)
			p.NetworkStatement()
		}

	case ShadowRustExtParserRESILIENCE:
		p.EnterOuterAlt(localctx, 9)
		{
			p.SetState(60)
			p.ResilienceStatement()
		}

	case ShadowRustExtParserACTIVATE:
		p.EnterOuterAlt(localctx, 10)
		{
			p.SetState(61)
			p.ActivateStatement()
		}

	case ShadowRustExtParserUPDATE_TRAIT:
		p.EnterOuterAlt(localctx, 11)
		{
			p.SetState(62)
			p.UpdateTraitStatement()
		}

	case ShadowRustExtParserVOTE:
		p.EnterOuterAlt(localctx, 12)
		{
			p.SetState(63)
			p.VoteStatement()
		}

	case ShadowRustExtParserSHARD:
		p.EnterOuterAlt(localctx, 13)
		{
			p.SetState(64)
			p.ShardStatement()
		}

	case ShadowRustExtParserASYNC_STAGGER:
		p.EnterOuterAlt(localctx, 14)
		{
			p.SetState(65)
			p.AsyncStaggerStatement()
		}

	case ShadowRustExtParserID:
		p.EnterOuterAlt(localctx, 15)
		{
			p.SetState(66)
			p.Assignment()
		}

	default:
		p.SetError(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		goto errorExit
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IContainerStatementContext is an interface to support dynamic dispatch.
type IContainerStatementContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	CONTAINER() antlr.TerminalNode
	AllContainerField() []IContainerFieldContext
	ContainerField(i int) IContainerFieldContext

	// IsContainerStatementContext differentiates from other interfaces.
	IsContainerStatementContext()
}

type ContainerStatementContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyContainerStatementContext() *ContainerStatementContext {
	var p = new(ContainerStatementContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_containerStatement
	return p
}

func InitEmptyContainerStatementContext(p *ContainerStatementContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_containerStatement
}

func (*ContainerStatementContext) IsContainerStatementContext() {}

func NewContainerStatementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ContainerStatementContext {
	var p = new(ContainerStatementContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ShadowRustExtParserRULE_containerStatement

	return p
}

func (s *ContainerStatementContext) GetParser() antlr.Parser { return s.parser }

func (s *ContainerStatementContext) CONTAINER() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserCONTAINER, 0)
}

func (s *ContainerStatementContext) AllContainerField() []IContainerFieldContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IContainerFieldContext); ok {
			len++
		}
	}

	tst := make([]IContainerFieldContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IContainerFieldContext); ok {
			tst[i] = t.(IContainerFieldContext)
			i++
		}
	}

	return tst
}

func (s *ContainerStatementContext) ContainerField(i int) IContainerFieldContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IContainerFieldContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IContainerFieldContext)
}

func (s *ContainerStatementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ContainerStatementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ContainerStatementContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.EnterContainerStatement(s)
	}
}

func (s *ContainerStatementContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.ExitContainerStatement(s)
	}
}

func (s *ContainerStatementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ShadowRustExtVisitor:
		return t.VisitContainerStatement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ShadowRustExtParser) ContainerStatement() (localctx IContainerStatementContext) {
	localctx = NewContainerStatementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 2, ShadowRustExtParserRULE_containerStatement)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(69)
		p.Match(ShadowRustExtParserCONTAINER)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(70)
		p.Match(ShadowRustExtParserT__0)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	p.SetState(74)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	for _la == ShadowRustExtParserID {
		{
			p.SetState(71)
			p.ContainerField()
		}

		p.SetState(76)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(77)
		p.Match(ShadowRustExtParserT__1)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IContainerFieldContext is an interface to support dynamic dispatch.
type IContainerFieldContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	ID() antlr.TerminalNode
	Expr() IExprContext

	// IsContainerFieldContext differentiates from other interfaces.
	IsContainerFieldContext()
}

type ContainerFieldContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyContainerFieldContext() *ContainerFieldContext {
	var p = new(ContainerFieldContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_containerField
	return p
}

func InitEmptyContainerFieldContext(p *ContainerFieldContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_containerField
}

func (*ContainerFieldContext) IsContainerFieldContext() {}

func NewContainerFieldContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ContainerFieldContext {
	var p = new(ContainerFieldContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ShadowRustExtParserRULE_containerField

	return p
}

func (s *ContainerFieldContext) GetParser() antlr.Parser { return s.parser }

func (s *ContainerFieldContext) ID() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserID, 0)
}

func (s *ContainerFieldContext) Expr() IExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExprContext)
}

func (s *ContainerFieldContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ContainerFieldContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ContainerFieldContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.EnterContainerField(s)
	}
}

func (s *ContainerFieldContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.ExitContainerField(s)
	}
}

func (s *ContainerFieldContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ShadowRustExtVisitor:
		return t.VisitContainerField(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ShadowRustExtParser) ContainerField() (localctx IContainerFieldContext) {
	localctx = NewContainerFieldContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 4, ShadowRustExtParserRULE_containerField)
	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(79)
		p.Match(ShadowRustExtParserID)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(80)
		p.Match(ShadowRustExtParserT__2)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(81)
		p.Expr()
	}
	{
		p.SetState(82)
		p.Match(ShadowRustExtParserT__3)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// INetworkStatementContext is an interface to support dynamic dispatch.
type INetworkStatementContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	NETWORK() antlr.TerminalNode
	AllNetworkField() []INetworkFieldContext
	NetworkField(i int) INetworkFieldContext

	// IsNetworkStatementContext differentiates from other interfaces.
	IsNetworkStatementContext()
}

type NetworkStatementContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyNetworkStatementContext() *NetworkStatementContext {
	var p = new(NetworkStatementContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_networkStatement
	return p
}

func InitEmptyNetworkStatementContext(p *NetworkStatementContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_networkStatement
}

func (*NetworkStatementContext) IsNetworkStatementContext() {}

func NewNetworkStatementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *NetworkStatementContext {
	var p = new(NetworkStatementContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ShadowRustExtParserRULE_networkStatement

	return p
}

func (s *NetworkStatementContext) GetParser() antlr.Parser { return s.parser }

func (s *NetworkStatementContext) NETWORK() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserNETWORK, 0)
}

func (s *NetworkStatementContext) AllNetworkField() []INetworkFieldContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(INetworkFieldContext); ok {
			len++
		}
	}

	tst := make([]INetworkFieldContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(INetworkFieldContext); ok {
			tst[i] = t.(INetworkFieldContext)
			i++
		}
	}

	return tst
}

func (s *NetworkStatementContext) NetworkField(i int) INetworkFieldContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(INetworkFieldContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(INetworkFieldContext)
}

func (s *NetworkStatementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *NetworkStatementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *NetworkStatementContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.EnterNetworkStatement(s)
	}
}

func (s *NetworkStatementContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.ExitNetworkStatement(s)
	}
}

func (s *NetworkStatementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ShadowRustExtVisitor:
		return t.VisitNetworkStatement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ShadowRustExtParser) NetworkStatement() (localctx INetworkStatementContext) {
	localctx = NewNetworkStatementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 6, ShadowRustExtParserRULE_networkStatement)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(84)
		p.Match(ShadowRustExtParserNETWORK)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(85)
		p.Match(ShadowRustExtParserT__0)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	p.SetState(89)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	for _la == ShadowRustExtParserID {
		{
			p.SetState(86)
			p.NetworkField()
		}

		p.SetState(91)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(92)
		p.Match(ShadowRustExtParserT__1)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// INetworkFieldContext is an interface to support dynamic dispatch.
type INetworkFieldContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	ID() antlr.TerminalNode
	Expr() IExprContext

	// IsNetworkFieldContext differentiates from other interfaces.
	IsNetworkFieldContext()
}

type NetworkFieldContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyNetworkFieldContext() *NetworkFieldContext {
	var p = new(NetworkFieldContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_networkField
	return p
}

func InitEmptyNetworkFieldContext(p *NetworkFieldContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_networkField
}

func (*NetworkFieldContext) IsNetworkFieldContext() {}

func NewNetworkFieldContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *NetworkFieldContext {
	var p = new(NetworkFieldContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ShadowRustExtParserRULE_networkField

	return p
}

func (s *NetworkFieldContext) GetParser() antlr.Parser { return s.parser }

func (s *NetworkFieldContext) ID() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserID, 0)
}

func (s *NetworkFieldContext) Expr() IExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExprContext)
}

func (s *NetworkFieldContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *NetworkFieldContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *NetworkFieldContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.EnterNetworkField(s)
	}
}

func (s *NetworkFieldContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.ExitNetworkField(s)
	}
}

func (s *NetworkFieldContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ShadowRustExtVisitor:
		return t.VisitNetworkField(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ShadowRustExtParser) NetworkField() (localctx INetworkFieldContext) {
	localctx = NewNetworkFieldContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 8, ShadowRustExtParserRULE_networkField)
	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(94)
		p.Match(ShadowRustExtParserID)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(95)
		p.Match(ShadowRustExtParserT__2)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(96)
		p.Expr()
	}
	{
		p.SetState(97)
		p.Match(ShadowRustExtParserT__3)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IResilienceStatementContext is an interface to support dynamic dispatch.
type IResilienceStatementContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	RESILIENCE() antlr.TerminalNode
	IF() antlr.TerminalNode
	Condition() IConditionContext
	AllStatement() []IStatementContext
	Statement(i int) IStatementContext

	// IsResilienceStatementContext differentiates from other interfaces.
	IsResilienceStatementContext()
}

type ResilienceStatementContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyResilienceStatementContext() *ResilienceStatementContext {
	var p = new(ResilienceStatementContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_resilienceStatement
	return p
}

func InitEmptyResilienceStatementContext(p *ResilienceStatementContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_resilienceStatement
}

func (*ResilienceStatementContext) IsResilienceStatementContext() {}

func NewResilienceStatementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ResilienceStatementContext {
	var p = new(ResilienceStatementContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ShadowRustExtParserRULE_resilienceStatement

	return p
}

func (s *ResilienceStatementContext) GetParser() antlr.Parser { return s.parser }

func (s *ResilienceStatementContext) RESILIENCE() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserRESILIENCE, 0)
}

func (s *ResilienceStatementContext) IF() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserIF, 0)
}

func (s *ResilienceStatementContext) Condition() IConditionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IConditionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IConditionContext)
}

func (s *ResilienceStatementContext) AllStatement() []IStatementContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IStatementContext); ok {
			len++
		}
	}

	tst := make([]IStatementContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IStatementContext); ok {
			tst[i] = t.(IStatementContext)
			i++
		}
	}

	return tst
}

func (s *ResilienceStatementContext) Statement(i int) IStatementContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IStatementContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IStatementContext)
}

func (s *ResilienceStatementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ResilienceStatementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ResilienceStatementContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.EnterResilienceStatement(s)
	}
}

func (s *ResilienceStatementContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.ExitResilienceStatement(s)
	}
}

func (s *ResilienceStatementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ShadowRustExtVisitor:
		return t.VisitResilienceStatement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ShadowRustExtParser) ResilienceStatement() (localctx IResilienceStatementContext) {
	localctx = NewResilienceStatementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 10, ShadowRustExtParserRULE_resilienceStatement)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(99)
		p.Match(ShadowRustExtParserRESILIENCE)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(100)
		p.Match(ShadowRustExtParserIF)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(101)
		p.Condition()
	}
	{
		p.SetState(102)
		p.Match(ShadowRustExtParserT__0)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	p.SetState(106)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	for (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&80643509452800) != 0 {
		{
			p.SetState(103)
			p.Statement()
		}

		p.SetState(108)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(109)
		p.Match(ShadowRustExtParserT__1)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IActivateStatementContext is an interface to support dynamic dispatch.
type IActivateStatementContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	ACTIVATE() antlr.TerminalNode
	SENTINELS() antlr.TerminalNode

	// IsActivateStatementContext differentiates from other interfaces.
	IsActivateStatementContext()
}

type ActivateStatementContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyActivateStatementContext() *ActivateStatementContext {
	var p = new(ActivateStatementContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_activateStatement
	return p
}

func InitEmptyActivateStatementContext(p *ActivateStatementContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_activateStatement
}

func (*ActivateStatementContext) IsActivateStatementContext() {}

func NewActivateStatementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ActivateStatementContext {
	var p = new(ActivateStatementContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ShadowRustExtParserRULE_activateStatement

	return p
}

func (s *ActivateStatementContext) GetParser() antlr.Parser { return s.parser }

func (s *ActivateStatementContext) ACTIVATE() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserACTIVATE, 0)
}

func (s *ActivateStatementContext) SENTINELS() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserSENTINELS, 0)
}

func (s *ActivateStatementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ActivateStatementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ActivateStatementContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.EnterActivateStatement(s)
	}
}

func (s *ActivateStatementContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.ExitActivateStatement(s)
	}
}

func (s *ActivateStatementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ShadowRustExtVisitor:
		return t.VisitActivateStatement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ShadowRustExtParser) ActivateStatement() (localctx IActivateStatementContext) {
	localctx = NewActivateStatementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 12, ShadowRustExtParserRULE_activateStatement)
	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(111)
		p.Match(ShadowRustExtParserACTIVATE)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(112)
		p.Match(ShadowRustExtParserSENTINELS)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(113)
		p.Match(ShadowRustExtParserT__3)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IUpdateTraitStatementContext is an interface to support dynamic dispatch.
type IUpdateTraitStatementContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// GetOp returns the op token.
	GetOp() antlr.Token

	// SetOp sets the op token.
	SetOp(antlr.Token)

	// Getter signatures
	UPDATE_TRAIT() antlr.TerminalNode
	AllID() []antlr.TerminalNode
	ID(i int) antlr.TerminalNode
	Expr() IExprContext
	PLUSEQ() antlr.TerminalNode
	MINUSEQ() antlr.TerminalNode

	// IsUpdateTraitStatementContext differentiates from other interfaces.
	IsUpdateTraitStatementContext()
}

type UpdateTraitStatementContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
	op     antlr.Token
}

func NewEmptyUpdateTraitStatementContext() *UpdateTraitStatementContext {
	var p = new(UpdateTraitStatementContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_updateTraitStatement
	return p
}

func InitEmptyUpdateTraitStatementContext(p *UpdateTraitStatementContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_updateTraitStatement
}

func (*UpdateTraitStatementContext) IsUpdateTraitStatementContext() {}

func NewUpdateTraitStatementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *UpdateTraitStatementContext {
	var p = new(UpdateTraitStatementContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ShadowRustExtParserRULE_updateTraitStatement

	return p
}

func (s *UpdateTraitStatementContext) GetParser() antlr.Parser { return s.parser }

func (s *UpdateTraitStatementContext) GetOp() antlr.Token { return s.op }

func (s *UpdateTraitStatementContext) SetOp(v antlr.Token) { s.op = v }

func (s *UpdateTraitStatementContext) UPDATE_TRAIT() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserUPDATE_TRAIT, 0)
}

func (s *UpdateTraitStatementContext) AllID() []antlr.TerminalNode {
	return s.GetTokens(ShadowRustExtParserID)
}

func (s *UpdateTraitStatementContext) ID(i int) antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserID, i)
}

func (s *UpdateTraitStatementContext) Expr() IExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExprContext)
}

func (s *UpdateTraitStatementContext) PLUSEQ() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserPLUSEQ, 0)
}

func (s *UpdateTraitStatementContext) MINUSEQ() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserMINUSEQ, 0)
}

func (s *UpdateTraitStatementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *UpdateTraitStatementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *UpdateTraitStatementContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.EnterUpdateTraitStatement(s)
	}
}

func (s *UpdateTraitStatementContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.ExitUpdateTraitStatement(s)
	}
}

func (s *UpdateTraitStatementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ShadowRustExtVisitor:
		return t.VisitUpdateTraitStatement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ShadowRustExtParser) UpdateTraitStatement() (localctx IUpdateTraitStatementContext) {
	localctx = NewUpdateTraitStatementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 14, ShadowRustExtParserRULE_updateTraitStatement)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(115)
		p.Match(ShadowRustExtParserUPDATE_TRAIT)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(116)
		p.Match(ShadowRustExtParserID)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(117)
		p.Match(ShadowRustExtParserID)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(118)

		var _lt = p.GetTokenStream().LT(1)

		localctx.(*UpdateTraitStatementContext).op = _lt

		_la = p.GetTokenStream().LA(1)

		if !((int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&805306376) != 0) {
			var _ri = p.GetErrorHandler().RecoverInline(p)

			localctx.(*UpdateTraitStatementContext).op = _ri
		} else {
			p.GetErrorHandler().ReportMatch(p)
			p.Consume()
		}
	}
	{
		p.SetState(119)
		p.Expr()
	}
	{
		p.SetState(120)
		p.Match(ShadowRustExtParserT__3)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IVoteStatementContext is an interface to support dynamic dispatch.
type IVoteStatementContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	VOTE() antlr.TerminalNode
	AllID() []antlr.TerminalNode
	ID(i int) antlr.TerminalNode

	// IsVoteStatementContext differentiates from other interfaces.
	IsVoteStatementContext()
}

type VoteStatementContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyVoteStatementContext() *VoteStatementContext {
	var p = new(VoteStatementContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_voteStatement
	return p
}

func InitEmptyVoteStatementContext(p *VoteStatementContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_voteStatement
}

func (*VoteStatementContext) IsVoteStatementContext() {}

func NewVoteStatementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *VoteStatementContext {
	var p = new(VoteStatementContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ShadowRustExtParserRULE_voteStatement

	return p
}

func (s *VoteStatementContext) GetParser() antlr.Parser { return s.parser }

func (s *VoteStatementContext) VOTE() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserVOTE, 0)
}

func (s *VoteStatementContext) AllID() []antlr.TerminalNode {
	return s.GetTokens(ShadowRustExtParserID)
}

func (s *VoteStatementContext) ID(i int) antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserID, i)
}

func (s *VoteStatementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *VoteStatementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *VoteStatementContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.EnterVoteStatement(s)
	}
}

func (s *VoteStatementContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.ExitVoteStatement(s)
	}
}

func (s *VoteStatementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ShadowRustExtVisitor:
		return t.VisitVoteStatement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ShadowRustExtParser) VoteStatement() (localctx IVoteStatementContext) {
	localctx = NewVoteStatementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 16, ShadowRustExtParserRULE_voteStatement)
	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(122)
		p.Match(ShadowRustExtParserVOTE)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(123)
		p.Match(ShadowRustExtParserID)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(124)
		p.Match(ShadowRustExtParserID)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(125)
		p.Match(ShadowRustExtParserT__3)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IShardStatementContext is an interface to support dynamic dispatch.
type IShardStatementContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	SHARD() antlr.TerminalNode
	ID() antlr.TerminalNode
	COUNT() antlr.TerminalNode
	Expr() IExprContext

	// IsShardStatementContext differentiates from other interfaces.
	IsShardStatementContext()
}

type ShardStatementContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyShardStatementContext() *ShardStatementContext {
	var p = new(ShardStatementContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_shardStatement
	return p
}

func InitEmptyShardStatementContext(p *ShardStatementContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_shardStatement
}

func (*ShardStatementContext) IsShardStatementContext() {}

func NewShardStatementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ShardStatementContext {
	var p = new(ShardStatementContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ShadowRustExtParserRULE_shardStatement

	return p
}

func (s *ShardStatementContext) GetParser() antlr.Parser { return s.parser }

func (s *ShardStatementContext) SHARD() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserSHARD, 0)
}

func (s *ShardStatementContext) ID() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserID, 0)
}

func (s *ShardStatementContext) COUNT() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserCOUNT, 0)
}

func (s *ShardStatementContext) Expr() IExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExprContext)
}

func (s *ShardStatementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ShardStatementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ShardStatementContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.EnterShardStatement(s)
	}
}

func (s *ShardStatementContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.ExitShardStatement(s)
	}
}

func (s *ShardStatementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ShadowRustExtVisitor:
		return t.VisitShardStatement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ShadowRustExtParser) ShardStatement() (localctx IShardStatementContext) {
	localctx = NewShardStatementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 18, ShadowRustExtParserRULE_shardStatement)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(127)
		p.Match(ShadowRustExtParserSHARD)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(128)
		p.Match(ShadowRustExtParserID)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	p.SetState(131)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	if _la == ShadowRustExtParserCOUNT {
		{
			p.SetState(129)
			p.Match(ShadowRustExtParserCOUNT)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		{
			p.SetState(130)
			p.Expr()
		}

	}
	{
		p.SetState(133)
		p.Match(ShadowRustExtParserT__3)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IAsyncStaggerStatementContext is an interface to support dynamic dispatch.
type IAsyncStaggerStatementContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	ASYNC_STAGGER() antlr.TerminalNode
	AllStaggerField() []IStaggerFieldContext
	StaggerField(i int) IStaggerFieldContext

	// IsAsyncStaggerStatementContext differentiates from other interfaces.
	IsAsyncStaggerStatementContext()
}

type AsyncStaggerStatementContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyAsyncStaggerStatementContext() *AsyncStaggerStatementContext {
	var p = new(AsyncStaggerStatementContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_asyncStaggerStatement
	return p
}

func InitEmptyAsyncStaggerStatementContext(p *AsyncStaggerStatementContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_asyncStaggerStatement
}

func (*AsyncStaggerStatementContext) IsAsyncStaggerStatementContext() {}

func NewAsyncStaggerStatementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *AsyncStaggerStatementContext {
	var p = new(AsyncStaggerStatementContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ShadowRustExtParserRULE_asyncStaggerStatement

	return p
}

func (s *AsyncStaggerStatementContext) GetParser() antlr.Parser { return s.parser }

func (s *AsyncStaggerStatementContext) ASYNC_STAGGER() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserASYNC_STAGGER, 0)
}

func (s *AsyncStaggerStatementContext) AllStaggerField() []IStaggerFieldContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IStaggerFieldContext); ok {
			len++
		}
	}

	tst := make([]IStaggerFieldContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IStaggerFieldContext); ok {
			tst[i] = t.(IStaggerFieldContext)
			i++
		}
	}

	return tst
}

func (s *AsyncStaggerStatementContext) StaggerField(i int) IStaggerFieldContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IStaggerFieldContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IStaggerFieldContext)
}

func (s *AsyncStaggerStatementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *AsyncStaggerStatementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *AsyncStaggerStatementContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.EnterAsyncStaggerStatement(s)
	}
}

func (s *AsyncStaggerStatementContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.ExitAsyncStaggerStatement(s)
	}
}

func (s *AsyncStaggerStatementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ShadowRustExtVisitor:
		return t.VisitAsyncStaggerStatement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ShadowRustExtParser) AsyncStaggerStatement() (localctx IAsyncStaggerStatementContext) {
	localctx = NewAsyncStaggerStatementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 20, ShadowRustExtParserRULE_asyncStaggerStatement)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(135)
		p.Match(ShadowRustExtParserASYNC_STAGGER)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(136)
		p.Match(ShadowRustExtParserT__0)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	p.SetState(140)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	for _la == ShadowRustExtParserID {
		{
			p.SetState(137)
			p.StaggerField()
		}

		p.SetState(142)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(143)
		p.Match(ShadowRustExtParserT__1)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IStaggerFieldContext is an interface to support dynamic dispatch.
type IStaggerFieldContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	ID() antlr.TerminalNode
	Expr() IExprContext

	// IsStaggerFieldContext differentiates from other interfaces.
	IsStaggerFieldContext()
}

type StaggerFieldContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyStaggerFieldContext() *StaggerFieldContext {
	var p = new(StaggerFieldContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_staggerField
	return p
}

func InitEmptyStaggerFieldContext(p *StaggerFieldContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_staggerField
}

func (*StaggerFieldContext) IsStaggerFieldContext() {}

func NewStaggerFieldContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *StaggerFieldContext {
	var p = new(StaggerFieldContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ShadowRustExtParserRULE_staggerField

	return p
}

func (s *StaggerFieldContext) GetParser() antlr.Parser { return s.parser }

func (s *StaggerFieldContext) ID() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserID, 0)
}

func (s *StaggerFieldContext) Expr() IExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExprContext)
}

func (s *StaggerFieldContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *StaggerFieldContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *StaggerFieldContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.EnterStaggerField(s)
	}
}

func (s *StaggerFieldContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.ExitStaggerField(s)
	}
}

func (s *StaggerFieldContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ShadowRustExtVisitor:
		return t.VisitStaggerField(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ShadowRustExtParser) StaggerField() (localctx IStaggerFieldContext) {
	localctx = NewStaggerFieldContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 22, ShadowRustExtParserRULE_staggerField)
	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(145)
		p.Match(ShadowRustExtParserID)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(146)
		p.Match(ShadowRustExtParserT__2)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(147)
		p.Expr()
	}
	{
		p.SetState(148)
		p.Match(ShadowRustExtParserT__3)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IFactorContext is an interface to support dynamic dispatch.
type IFactorContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	ID() antlr.TerminalNode
	AMOUNT() antlr.TerminalNode
	NUMBER() antlr.TerminalNode
	Expr() IExprContext

	// IsFactorContext differentiates from other interfaces.
	IsFactorContext()
}

type FactorContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyFactorContext() *FactorContext {
	var p = new(FactorContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_factor
	return p
}

func InitEmptyFactorContext(p *FactorContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_factor
}

func (*FactorContext) IsFactorContext() {}

func NewFactorContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *FactorContext {
	var p = new(FactorContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ShadowRustExtParserRULE_factor

	return p
}

func (s *FactorContext) GetParser() antlr.Parser { return s.parser }

func (s *FactorContext) ID() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserID, 0)
}

func (s *FactorContext) AMOUNT() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserAMOUNT, 0)
}

func (s *FactorContext) NUMBER() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserNUMBER, 0)
}

func (s *FactorContext) Expr() IExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExprContext)
}

func (s *FactorContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *FactorContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *FactorContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.EnterFactor(s)
	}
}

func (s *FactorContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.ExitFactor(s)
	}
}

func (s *FactorContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ShadowRustExtVisitor:
		return t.VisitFactor(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ShadowRustExtParser) Factor() (localctx IFactorContext) {
	localctx = NewFactorContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 24, ShadowRustExtParserRULE_factor)
	p.SetState(157)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}

	switch p.GetTokenStream().LA(1) {
	case ShadowRustExtParserID:
		p.EnterOuterAlt(localctx, 1)
		{
			p.SetState(150)
			p.Match(ShadowRustExtParserID)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	case ShadowRustExtParserAMOUNT:
		p.EnterOuterAlt(localctx, 2)
		{
			p.SetState(151)
			p.Match(ShadowRustExtParserAMOUNT)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	case ShadowRustExtParserNUMBER:
		p.EnterOuterAlt(localctx, 3)
		{
			p.SetState(152)
			p.Match(ShadowRustExtParserNUMBER)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	case ShadowRustExtParserT__4:
		p.EnterOuterAlt(localctx, 4)
		{
			p.SetState(153)
			p.Match(ShadowRustExtParserT__4)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		{
			p.SetState(154)
			p.Expr()
		}
		{
			p.SetState(155)
			p.Match(ShadowRustExtParserT__5)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	default:
		p.SetError(antlr.NewNoViableAltException(p, nil, nil, nil, nil, nil))
		goto errorExit
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IProgramContext is an interface to support dynamic dispatch.
type IProgramContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	EOF() antlr.TerminalNode
	AllStatement() []IStatementContext
	Statement(i int) IStatementContext

	// IsProgramContext differentiates from other interfaces.
	IsProgramContext()
}

type ProgramContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyProgramContext() *ProgramContext {
	var p = new(ProgramContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_program
	return p
}

func InitEmptyProgramContext(p *ProgramContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_program
}

func (*ProgramContext) IsProgramContext() {}

func NewProgramContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ProgramContext {
	var p = new(ProgramContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ShadowRustExtParserRULE_program

	return p
}

func (s *ProgramContext) GetParser() antlr.Parser { return s.parser }

func (s *ProgramContext) EOF() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserEOF, 0)
}

func (s *ProgramContext) AllStatement() []IStatementContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IStatementContext); ok {
			len++
		}
	}

	tst := make([]IStatementContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IStatementContext); ok {
			tst[i] = t.(IStatementContext)
			i++
		}
	}

	return tst
}

func (s *ProgramContext) Statement(i int) IStatementContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IStatementContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IStatementContext)
}

func (s *ProgramContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ProgramContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ProgramContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.EnterProgram(s)
	}
}

func (s *ProgramContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.ExitProgram(s)
	}
}

func (s *ProgramContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ShadowRustExtVisitor:
		return t.VisitProgram(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ShadowRustExtParser) Program() (localctx IProgramContext) {
	localctx = NewProgramContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 26, ShadowRustExtParserRULE_program)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	p.SetState(162)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	for (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&80643509452800) != 0 {
		{
			p.SetState(159)
			p.Statement()
		}

		p.SetState(164)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(165)
		p.Match(ShadowRustExtParserEOF)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IIfStatementContext is an interface to support dynamic dispatch.
type IIfStatementContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	IF() antlr.TerminalNode
	Condition() IConditionContext
	AllStatement() []IStatementContext
	Statement(i int) IStatementContext

	// IsIfStatementContext differentiates from other interfaces.
	IsIfStatementContext()
}

type IfStatementContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyIfStatementContext() *IfStatementContext {
	var p = new(IfStatementContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_ifStatement
	return p
}

func InitEmptyIfStatementContext(p *IfStatementContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_ifStatement
}

func (*IfStatementContext) IsIfStatementContext() {}

func NewIfStatementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *IfStatementContext {
	var p = new(IfStatementContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ShadowRustExtParserRULE_ifStatement

	return p
}

func (s *IfStatementContext) GetParser() antlr.Parser { return s.parser }

func (s *IfStatementContext) IF() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserIF, 0)
}

func (s *IfStatementContext) Condition() IConditionContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IConditionContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IConditionContext)
}

func (s *IfStatementContext) AllStatement() []IStatementContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IStatementContext); ok {
			len++
		}
	}

	tst := make([]IStatementContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IStatementContext); ok {
			tst[i] = t.(IStatementContext)
			i++
		}
	}

	return tst
}

func (s *IfStatementContext) Statement(i int) IStatementContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IStatementContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IStatementContext)
}

func (s *IfStatementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *IfStatementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *IfStatementContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.EnterIfStatement(s)
	}
}

func (s *IfStatementContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.ExitIfStatement(s)
	}
}

func (s *IfStatementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ShadowRustExtVisitor:
		return t.VisitIfStatement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ShadowRustExtParser) IfStatement() (localctx IIfStatementContext) {
	localctx = NewIfStatementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 28, ShadowRustExtParserRULE_ifStatement)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(167)
		p.Match(ShadowRustExtParserIF)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(168)
		p.Condition()
	}
	{
		p.SetState(169)
		p.Match(ShadowRustExtParserT__0)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	p.SetState(173)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	for (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&80643509452800) != 0 {
		{
			p.SetState(170)
			p.Statement()
		}

		p.SetState(175)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(176)
		p.Match(ShadowRustExtParserT__1)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// ITxStatementContext is an interface to support dynamic dispatch.
type ITxStatementContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	TX() antlr.TerminalNode
	BUY() antlr.TerminalNode
	AllID() []antlr.TerminalNode
	ID(i int) antlr.TerminalNode
	FROM() antlr.TerminalNode
	TO() antlr.TerminalNode
	AMOUNT() antlr.TerminalNode
	Expr() IExprContext
	AllStatement() []IStatementContext
	Statement(i int) IStatementContext

	// IsTxStatementContext differentiates from other interfaces.
	IsTxStatementContext()
}

type TxStatementContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyTxStatementContext() *TxStatementContext {
	var p = new(TxStatementContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_txStatement
	return p
}

func InitEmptyTxStatementContext(p *TxStatementContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_txStatement
}

func (*TxStatementContext) IsTxStatementContext() {}

func NewTxStatementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *TxStatementContext {
	var p = new(TxStatementContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ShadowRustExtParserRULE_txStatement

	return p
}

func (s *TxStatementContext) GetParser() antlr.Parser { return s.parser }

func (s *TxStatementContext) TX() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserTX, 0)
}

func (s *TxStatementContext) BUY() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserBUY, 0)
}

func (s *TxStatementContext) AllID() []antlr.TerminalNode {
	return s.GetTokens(ShadowRustExtParserID)
}

func (s *TxStatementContext) ID(i int) antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserID, i)
}

func (s *TxStatementContext) FROM() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserFROM, 0)
}

func (s *TxStatementContext) TO() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserTO, 0)
}

func (s *TxStatementContext) AMOUNT() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserAMOUNT, 0)
}

func (s *TxStatementContext) Expr() IExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExprContext)
}

func (s *TxStatementContext) AllStatement() []IStatementContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IStatementContext); ok {
			len++
		}
	}

	tst := make([]IStatementContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IStatementContext); ok {
			tst[i] = t.(IStatementContext)
			i++
		}
	}

	return tst
}

func (s *TxStatementContext) Statement(i int) IStatementContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IStatementContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IStatementContext)
}

func (s *TxStatementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *TxStatementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *TxStatementContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.EnterTxStatement(s)
	}
}

func (s *TxStatementContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.ExitTxStatement(s)
	}
}

func (s *TxStatementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ShadowRustExtVisitor:
		return t.VisitTxStatement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ShadowRustExtParser) TxStatement() (localctx ITxStatementContext) {
	localctx = NewTxStatementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 30, ShadowRustExtParserRULE_txStatement)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(178)
		p.Match(ShadowRustExtParserTX)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(179)
		p.Match(ShadowRustExtParserBUY)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(180)
		p.Match(ShadowRustExtParserID)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(181)
		p.Match(ShadowRustExtParserFROM)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(182)
		p.Match(ShadowRustExtParserID)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(183)
		p.Match(ShadowRustExtParserTO)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(184)
		p.Match(ShadowRustExtParserID)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(185)
		p.Match(ShadowRustExtParserAMOUNT)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(186)
		p.Expr()
	}
	{
		p.SetState(187)
		p.Match(ShadowRustExtParserT__0)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	p.SetState(191)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	for (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&80643509452800) != 0 {
		{
			p.SetState(188)
			p.Statement()
		}

		p.SetState(193)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(194)
		p.Match(ShadowRustExtParserT__1)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IMintStatementContext is an interface to support dynamic dispatch.
type IMintStatementContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	MINT() antlr.TerminalNode
	ID() antlr.TerminalNode
	AMOUNT() antlr.TerminalNode
	AllExpr() []IExprContext
	Expr(i int) IExprContext
	EPOCH() antlr.TerminalNode

	// IsMintStatementContext differentiates from other interfaces.
	IsMintStatementContext()
}

type MintStatementContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyMintStatementContext() *MintStatementContext {
	var p = new(MintStatementContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_mintStatement
	return p
}

func InitEmptyMintStatementContext(p *MintStatementContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_mintStatement
}

func (*MintStatementContext) IsMintStatementContext() {}

func NewMintStatementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *MintStatementContext {
	var p = new(MintStatementContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ShadowRustExtParserRULE_mintStatement

	return p
}

func (s *MintStatementContext) GetParser() antlr.Parser { return s.parser }

func (s *MintStatementContext) MINT() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserMINT, 0)
}

func (s *MintStatementContext) ID() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserID, 0)
}

func (s *MintStatementContext) AMOUNT() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserAMOUNT, 0)
}

func (s *MintStatementContext) AllExpr() []IExprContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IExprContext); ok {
			len++
		}
	}

	tst := make([]IExprContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IExprContext); ok {
			tst[i] = t.(IExprContext)
			i++
		}
	}

	return tst
}

func (s *MintStatementContext) Expr(i int) IExprContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExprContext)
}

func (s *MintStatementContext) EPOCH() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserEPOCH, 0)
}

func (s *MintStatementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *MintStatementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *MintStatementContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.EnterMintStatement(s)
	}
}

func (s *MintStatementContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.ExitMintStatement(s)
	}
}

func (s *MintStatementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ShadowRustExtVisitor:
		return t.VisitMintStatement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ShadowRustExtParser) MintStatement() (localctx IMintStatementContext) {
	localctx = NewMintStatementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 32, ShadowRustExtParserRULE_mintStatement)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(196)
		p.Match(ShadowRustExtParserMINT)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(197)
		p.Match(ShadowRustExtParserID)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(198)
		p.Match(ShadowRustExtParserAMOUNT)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(199)
		p.Expr()
	}
	p.SetState(202)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	if _la == ShadowRustExtParserEPOCH {
		{
			p.SetState(200)
			p.Match(ShadowRustExtParserEPOCH)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		{
			p.SetState(201)
			p.Expr()
		}

	}
	{
		p.SetState(204)
		p.Match(ShadowRustExtParserT__3)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IValidateStatementContext is an interface to support dynamic dispatch.
type IValidateStatementContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	VALIDATE() antlr.TerminalNode
	ID() antlr.TerminalNode
	STAGE() antlr.TerminalNode
	NUMBER() antlr.TerminalNode
	AllStatement() []IStatementContext
	Statement(i int) IStatementContext

	// IsValidateStatementContext differentiates from other interfaces.
	IsValidateStatementContext()
}

type ValidateStatementContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyValidateStatementContext() *ValidateStatementContext {
	var p = new(ValidateStatementContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_validateStatement
	return p
}

func InitEmptyValidateStatementContext(p *ValidateStatementContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_validateStatement
}

func (*ValidateStatementContext) IsValidateStatementContext() {}

func NewValidateStatementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ValidateStatementContext {
	var p = new(ValidateStatementContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ShadowRustExtParserRULE_validateStatement

	return p
}

func (s *ValidateStatementContext) GetParser() antlr.Parser { return s.parser }

func (s *ValidateStatementContext) VALIDATE() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserVALIDATE, 0)
}

func (s *ValidateStatementContext) ID() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserID, 0)
}

func (s *ValidateStatementContext) STAGE() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserSTAGE, 0)
}

func (s *ValidateStatementContext) NUMBER() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserNUMBER, 0)
}

func (s *ValidateStatementContext) AllStatement() []IStatementContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IStatementContext); ok {
			len++
		}
	}

	tst := make([]IStatementContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IStatementContext); ok {
			tst[i] = t.(IStatementContext)
			i++
		}
	}

	return tst
}

func (s *ValidateStatementContext) Statement(i int) IStatementContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IStatementContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IStatementContext)
}

func (s *ValidateStatementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ValidateStatementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ValidateStatementContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.EnterValidateStatement(s)
	}
}

func (s *ValidateStatementContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.ExitValidateStatement(s)
	}
}

func (s *ValidateStatementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ShadowRustExtVisitor:
		return t.VisitValidateStatement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ShadowRustExtParser) ValidateStatement() (localctx IValidateStatementContext) {
	localctx = NewValidateStatementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 34, ShadowRustExtParserRULE_validateStatement)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(206)
		p.Match(ShadowRustExtParserVALIDATE)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(207)
		p.Match(ShadowRustExtParserID)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(208)
		p.Match(ShadowRustExtParserSTAGE)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(209)
		p.Match(ShadowRustExtParserNUMBER)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(210)
		p.Match(ShadowRustExtParserT__0)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	p.SetState(214)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	for (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&80643509452800) != 0 {
		{
			p.SetState(211)
			p.Statement()
		}

		p.SetState(216)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(217)
		p.Match(ShadowRustExtParserT__1)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IQueueStatementContext is an interface to support dynamic dispatch.
type IQueueStatementContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	QUEUE() antlr.TerminalNode
	INSERT() antlr.TerminalNode
	ID() antlr.TerminalNode
	POSITIONS() antlr.TerminalNode
	AllExpr() []IExprContext
	Expr(i int) IExprContext

	// IsQueueStatementContext differentiates from other interfaces.
	IsQueueStatementContext()
}

type QueueStatementContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyQueueStatementContext() *QueueStatementContext {
	var p = new(QueueStatementContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_queueStatement
	return p
}

func InitEmptyQueueStatementContext(p *QueueStatementContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_queueStatement
}

func (*QueueStatementContext) IsQueueStatementContext() {}

func NewQueueStatementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *QueueStatementContext {
	var p = new(QueueStatementContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ShadowRustExtParserRULE_queueStatement

	return p
}

func (s *QueueStatementContext) GetParser() antlr.Parser { return s.parser }

func (s *QueueStatementContext) QUEUE() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserQUEUE, 0)
}

func (s *QueueStatementContext) INSERT() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserINSERT, 0)
}

func (s *QueueStatementContext) ID() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserID, 0)
}

func (s *QueueStatementContext) POSITIONS() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserPOSITIONS, 0)
}

func (s *QueueStatementContext) AllExpr() []IExprContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IExprContext); ok {
			len++
		}
	}

	tst := make([]IExprContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IExprContext); ok {
			tst[i] = t.(IExprContext)
			i++
		}
	}

	return tst
}

func (s *QueueStatementContext) Expr(i int) IExprContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExprContext)
}

func (s *QueueStatementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *QueueStatementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *QueueStatementContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.EnterQueueStatement(s)
	}
}

func (s *QueueStatementContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.ExitQueueStatement(s)
	}
}

func (s *QueueStatementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ShadowRustExtVisitor:
		return t.VisitQueueStatement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ShadowRustExtParser) QueueStatement() (localctx IQueueStatementContext) {
	localctx = NewQueueStatementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 36, ShadowRustExtParserRULE_queueStatement)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(219)
		p.Match(ShadowRustExtParserQUEUE)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(220)
		p.Match(ShadowRustExtParserINSERT)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(221)
		p.Match(ShadowRustExtParserID)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(222)
		p.Match(ShadowRustExtParserPOSITIONS)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(223)
		p.Expr()
	}
	p.SetState(228)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	for _la == ShadowRustExtParserT__6 {
		{
			p.SetState(224)
			p.Match(ShadowRustExtParserT__6)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		{
			p.SetState(225)
			p.Expr()
		}

		p.SetState(230)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)
	}
	{
		p.SetState(231)
		p.Match(ShadowRustExtParserT__3)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IBankStatementContext is an interface to support dynamic dispatch.
type IBankStatementContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	BANK() antlr.TerminalNode
	DEPOSIT() antlr.TerminalNode
	ID() antlr.TerminalNode
	ATR() antlr.TerminalNode
	Expr() IExprContext

	// IsBankStatementContext differentiates from other interfaces.
	IsBankStatementContext()
}

type BankStatementContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyBankStatementContext() *BankStatementContext {
	var p = new(BankStatementContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_bankStatement
	return p
}

func InitEmptyBankStatementContext(p *BankStatementContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_bankStatement
}

func (*BankStatementContext) IsBankStatementContext() {}

func NewBankStatementContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *BankStatementContext {
	var p = new(BankStatementContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ShadowRustExtParserRULE_bankStatement

	return p
}

func (s *BankStatementContext) GetParser() antlr.Parser { return s.parser }

func (s *BankStatementContext) BANK() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserBANK, 0)
}

func (s *BankStatementContext) DEPOSIT() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserDEPOSIT, 0)
}

func (s *BankStatementContext) ID() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserID, 0)
}

func (s *BankStatementContext) ATR() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserATR, 0)
}

func (s *BankStatementContext) Expr() IExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExprContext)
}

func (s *BankStatementContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *BankStatementContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *BankStatementContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.EnterBankStatement(s)
	}
}

func (s *BankStatementContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.ExitBankStatement(s)
	}
}

func (s *BankStatementContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ShadowRustExtVisitor:
		return t.VisitBankStatement(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ShadowRustExtParser) BankStatement() (localctx IBankStatementContext) {
	localctx = NewBankStatementContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 38, ShadowRustExtParserRULE_bankStatement)
	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(233)
		p.Match(ShadowRustExtParserBANK)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(234)
		p.Match(ShadowRustExtParserDEPOSIT)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(235)
		p.Match(ShadowRustExtParserID)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(236)
		p.Match(ShadowRustExtParserATR)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(237)
		p.Expr()
	}
	{
		p.SetState(238)
		p.Match(ShadowRustExtParserT__3)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IAssignmentContext is an interface to support dynamic dispatch.
type IAssignmentContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	ID() antlr.TerminalNode
	Expr() IExprContext

	// IsAssignmentContext differentiates from other interfaces.
	IsAssignmentContext()
}

type AssignmentContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyAssignmentContext() *AssignmentContext {
	var p = new(AssignmentContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_assignment
	return p
}

func InitEmptyAssignmentContext(p *AssignmentContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_assignment
}

func (*AssignmentContext) IsAssignmentContext() {}

func NewAssignmentContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *AssignmentContext {
	var p = new(AssignmentContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ShadowRustExtParserRULE_assignment

	return p
}

func (s *AssignmentContext) GetParser() antlr.Parser { return s.parser }

func (s *AssignmentContext) ID() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserID, 0)
}

func (s *AssignmentContext) Expr() IExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExprContext)
}

func (s *AssignmentContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *AssignmentContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *AssignmentContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.EnterAssignment(s)
	}
}

func (s *AssignmentContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.ExitAssignment(s)
	}
}

func (s *AssignmentContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ShadowRustExtVisitor:
		return t.VisitAssignment(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ShadowRustExtParser) Assignment() (localctx IAssignmentContext) {
	localctx = NewAssignmentContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 40, ShadowRustExtParserRULE_assignment)
	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(240)
		p.Match(ShadowRustExtParserID)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(241)
		p.Match(ShadowRustExtParserT__2)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}
	{
		p.SetState(242)
		p.Expr()
	}
	{
		p.SetState(243)
		p.Match(ShadowRustExtParserT__3)
		if p.HasError() {
			// Recognition error - abort rule
			goto errorExit
		}
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IConditionContext is an interface to support dynamic dispatch.
type IConditionContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	Expr() IExprContext

	// IsConditionContext differentiates from other interfaces.
	IsConditionContext()
}

type ConditionContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyConditionContext() *ConditionContext {
	var p = new(ConditionContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_condition
	return p
}

func InitEmptyConditionContext(p *ConditionContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_condition
}

func (*ConditionContext) IsConditionContext() {}

func NewConditionContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ConditionContext {
	var p = new(ConditionContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ShadowRustExtParserRULE_condition

	return p
}

func (s *ConditionContext) GetParser() antlr.Parser { return s.parser }

func (s *ConditionContext) Expr() IExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IExprContext)
}

func (s *ConditionContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ConditionContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ConditionContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.EnterCondition(s)
	}
}

func (s *ConditionContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.ExitCondition(s)
	}
}

func (s *ConditionContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ShadowRustExtVisitor:
		return t.VisitCondition(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ShadowRustExtParser) Condition() (localctx IConditionContext) {
	localctx = NewConditionContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 42, ShadowRustExtParserRULE_condition)
	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(245)
		p.Expr()
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IExprContext is an interface to support dynamic dispatch.
type IExprContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// Getter signatures
	RelExpr() IRelExprContext
	TO() antlr.TerminalNode
	ID() antlr.TerminalNode

	// IsExprContext differentiates from other interfaces.
	IsExprContext()
}

type ExprContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
}

func NewEmptyExprContext() *ExprContext {
	var p = new(ExprContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_expr
	return p
}

func InitEmptyExprContext(p *ExprContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_expr
}

func (*ExprContext) IsExprContext() {}

func NewExprContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ExprContext {
	var p = new(ExprContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ShadowRustExtParserRULE_expr

	return p
}

func (s *ExprContext) GetParser() antlr.Parser { return s.parser }

func (s *ExprContext) RelExpr() IRelExprContext {
	var t antlr.RuleContext
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IRelExprContext); ok {
			t = ctx.(antlr.RuleContext)
			break
		}
	}

	if t == nil {
		return nil
	}

	return t.(IRelExprContext)
}

func (s *ExprContext) TO() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserTO, 0)
}

func (s *ExprContext) ID() antlr.TerminalNode {
	return s.GetToken(ShadowRustExtParserID, 0)
}

func (s *ExprContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ExprContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ExprContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.EnterExpr(s)
	}
}

func (s *ExprContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.ExitExpr(s)
	}
}

func (s *ExprContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ShadowRustExtVisitor:
		return t.VisitExpr(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ShadowRustExtParser) Expr() (localctx IExprContext) {
	localctx = NewExprContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 44, ShadowRustExtParserRULE_expr)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(247)
		p.RelExpr()
	}
	p.SetState(250)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	if _la == ShadowRustExtParserTO {
		{
			p.SetState(248)
			p.Match(ShadowRustExtParserTO)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}
		{
			p.SetState(249)
			p.Match(ShadowRustExtParserID)
			if p.HasError() {
				// Recognition error - abort rule
				goto errorExit
			}
		}

	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IRelExprContext is an interface to support dynamic dispatch.
type IRelExprContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// GetOp returns the op token.
	GetOp() antlr.Token

	// SetOp sets the op token.
	SetOp(antlr.Token)

	// Getter signatures
	AllArithExpr() []IArithExprContext
	ArithExpr(i int) IArithExprContext

	// IsRelExprContext differentiates from other interfaces.
	IsRelExprContext()
}

type RelExprContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
	op     antlr.Token
}

func NewEmptyRelExprContext() *RelExprContext {
	var p = new(RelExprContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_relExpr
	return p
}

func InitEmptyRelExprContext(p *RelExprContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_relExpr
}

func (*RelExprContext) IsRelExprContext() {}

func NewRelExprContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *RelExprContext {
	var p = new(RelExprContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ShadowRustExtParserRULE_relExpr

	return p
}

func (s *RelExprContext) GetParser() antlr.Parser { return s.parser }

func (s *RelExprContext) GetOp() antlr.Token { return s.op }

func (s *RelExprContext) SetOp(v antlr.Token) { s.op = v }

func (s *RelExprContext) AllArithExpr() []IArithExprContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IArithExprContext); ok {
			len++
		}
	}

	tst := make([]IArithExprContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IArithExprContext); ok {
			tst[i] = t.(IArithExprContext)
			i++
		}
	}

	return tst
}

func (s *RelExprContext) ArithExpr(i int) IArithExprContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IArithExprContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IArithExprContext)
}

func (s *RelExprContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *RelExprContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *RelExprContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.EnterRelExpr(s)
	}
}

func (s *RelExprContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.ExitRelExpr(s)
	}
}

func (s *RelExprContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ShadowRustExtVisitor:
		return t.VisitRelExpr(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ShadowRustExtParser) RelExpr() (localctx IRelExprContext) {
	localctx = NewRelExprContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 46, ShadowRustExtParserRULE_relExpr)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(252)
		p.ArithExpr()
	}
	p.SetState(257)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	for (int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&16128) != 0 {
		{
			p.SetState(253)

			var _lt = p.GetTokenStream().LT(1)

			localctx.(*RelExprContext).op = _lt

			_la = p.GetTokenStream().LA(1)

			if !((int64(_la) & ^0x3f) == 0 && ((int64(1)<<_la)&16128) != 0) {
				var _ri = p.GetErrorHandler().RecoverInline(p)

				localctx.(*RelExprContext).op = _ri
			} else {
				p.GetErrorHandler().ReportMatch(p)
				p.Consume()
			}
		}
		{
			p.SetState(254)
			p.ArithExpr()
		}

		p.SetState(259)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// IArithExprContext is an interface to support dynamic dispatch.
type IArithExprContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// GetOp returns the op token.
	GetOp() antlr.Token

	// SetOp sets the op token.
	SetOp(antlr.Token)

	// Getter signatures
	AllTerm() []ITermContext
	Term(i int) ITermContext

	// IsArithExprContext differentiates from other interfaces.
	IsArithExprContext()
}

type ArithExprContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
	op     antlr.Token
}

func NewEmptyArithExprContext() *ArithExprContext {
	var p = new(ArithExprContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_arithExpr
	return p
}

func InitEmptyArithExprContext(p *ArithExprContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_arithExpr
}

func (*ArithExprContext) IsArithExprContext() {}

func NewArithExprContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *ArithExprContext {
	var p = new(ArithExprContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ShadowRustExtParserRULE_arithExpr

	return p
}

func (s *ArithExprContext) GetParser() antlr.Parser { return s.parser }

func (s *ArithExprContext) GetOp() antlr.Token { return s.op }

func (s *ArithExprContext) SetOp(v antlr.Token) { s.op = v }

func (s *ArithExprContext) AllTerm() []ITermContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(ITermContext); ok {
			len++
		}
	}

	tst := make([]ITermContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(ITermContext); ok {
			tst[i] = t.(ITermContext)
			i++
		}
	}

	return tst
}

func (s *ArithExprContext) Term(i int) ITermContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(ITermContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(ITermContext)
}

func (s *ArithExprContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *ArithExprContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *ArithExprContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.EnterArithExpr(s)
	}
}

func (s *ArithExprContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.ExitArithExpr(s)
	}
}

func (s *ArithExprContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ShadowRustExtVisitor:
		return t.VisitArithExpr(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ShadowRustExtParser) ArithExpr() (localctx IArithExprContext) {
	localctx = NewArithExprContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 48, ShadowRustExtParserRULE_arithExpr)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(260)
		p.Term()
	}
	p.SetState(265)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	for _la == ShadowRustExtParserT__13 || _la == ShadowRustExtParserT__14 {
		{
			p.SetState(261)

			var _lt = p.GetTokenStream().LT(1)

			localctx.(*ArithExprContext).op = _lt

			_la = p.GetTokenStream().LA(1)

			if !(_la == ShadowRustExtParserT__13 || _la == ShadowRustExtParserT__14) {
				var _ri = p.GetErrorHandler().RecoverInline(p)

				localctx.(*ArithExprContext).op = _ri
			} else {
				p.GetErrorHandler().ReportMatch(p)
				p.Consume()
			}
		}
		{
			p.SetState(262)
			p.Term()
		}

		p.SetState(267)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}

// ITermContext is an interface to support dynamic dispatch.
type ITermContext interface {
	antlr.ParserRuleContext

	// GetParser returns the parser.
	GetParser() antlr.Parser

	// GetOp returns the op token.
	GetOp() antlr.Token

	// SetOp sets the op token.
	SetOp(antlr.Token)

	// Getter signatures
	AllFactor() []IFactorContext
	Factor(i int) IFactorContext

	// IsTermContext differentiates from other interfaces.
	IsTermContext()
}

type TermContext struct {
	antlr.BaseParserRuleContext
	parser antlr.Parser
	op     antlr.Token
}

func NewEmptyTermContext() *TermContext {
	var p = new(TermContext)
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_term
	return p
}

func InitEmptyTermContext(p *TermContext) {
	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, nil, -1)
	p.RuleIndex = ShadowRustExtParserRULE_term
}

func (*TermContext) IsTermContext() {}

func NewTermContext(parser antlr.Parser, parent antlr.ParserRuleContext, invokingState int) *TermContext {
	var p = new(TermContext)

	antlr.InitBaseParserRuleContext(&p.BaseParserRuleContext, parent, invokingState)

	p.parser = parser
	p.RuleIndex = ShadowRustExtParserRULE_term

	return p
}

func (s *TermContext) GetParser() antlr.Parser { return s.parser }

func (s *TermContext) GetOp() antlr.Token { return s.op }

func (s *TermContext) SetOp(v antlr.Token) { s.op = v }

func (s *TermContext) AllFactor() []IFactorContext {
	children := s.GetChildren()
	len := 0
	for _, ctx := range children {
		if _, ok := ctx.(IFactorContext); ok {
			len++
		}
	}

	tst := make([]IFactorContext, len)
	i := 0
	for _, ctx := range children {
		if t, ok := ctx.(IFactorContext); ok {
			tst[i] = t.(IFactorContext)
			i++
		}
	}

	return tst
}

func (s *TermContext) Factor(i int) IFactorContext {
	var t antlr.RuleContext
	j := 0
	for _, ctx := range s.GetChildren() {
		if _, ok := ctx.(IFactorContext); ok {
			if j == i {
				t = ctx.(antlr.RuleContext)
				break
			}
			j++
		}
	}

	if t == nil {
		return nil
	}

	return t.(IFactorContext)
}

func (s *TermContext) GetRuleContext() antlr.RuleContext {
	return s
}

func (s *TermContext) ToStringTree(ruleNames []string, recog antlr.Recognizer) string {
	return antlr.TreesStringTree(s, ruleNames, recog)
}

func (s *TermContext) EnterRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.EnterTerm(s)
	}
}

func (s *TermContext) ExitRule(listener antlr.ParseTreeListener) {
	if listenerT, ok := listener.(ShadowRustExtListener); ok {
		listenerT.ExitTerm(s)
	}
}

func (s *TermContext) Accept(visitor antlr.ParseTreeVisitor) interface{} {
	switch t := visitor.(type) {
	case ShadowRustExtVisitor:
		return t.VisitTerm(s)

	default:
		return t.VisitChildren(s)
	}
}

func (p *ShadowRustExtParser) Term() (localctx ITermContext) {
	localctx = NewTermContext(p, p.GetParserRuleContext(), p.GetState())
	p.EnterRule(localctx, 50, ShadowRustExtParserRULE_term)
	var _la int

	p.EnterOuterAlt(localctx, 1)
	{
		p.SetState(268)
		p.Factor()
	}
	p.SetState(273)
	p.GetErrorHandler().Sync(p)
	if p.HasError() {
		goto errorExit
	}
	_la = p.GetTokenStream().LA(1)

	for _la == ShadowRustExtParserT__15 || _la == ShadowRustExtParserT__16 {
		{
			p.SetState(269)

			var _lt = p.GetTokenStream().LT(1)

			localctx.(*TermContext).op = _lt

			_la = p.GetTokenStream().LA(1)

			if !(_la == ShadowRustExtParserT__15 || _la == ShadowRustExtParserT__16) {
				var _ri = p.GetErrorHandler().RecoverInline(p)

				localctx.(*TermContext).op = _ri
			} else {
				p.GetErrorHandler().ReportMatch(p)
				p.Consume()
			}
		}
		{
			p.SetState(270)
			p.Factor()
		}

		p.SetState(275)
		p.GetErrorHandler().Sync(p)
		if p.HasError() {
			goto errorExit
		}
		_la = p.GetTokenStream().LA(1)
	}

errorExit:
	if p.HasError() {
		v := p.GetError()
		localctx.SetException(v)
		p.GetErrorHandler().ReportError(p, v)
		p.GetErrorHandler().Recover(p, v)
		p.SetError(nil)
	}
	p.ExitRule()
	return localctx
	goto errorExit // Trick to prevent compiler error if the label is not used
}
