// ShadowRust core grammar (v1), canonical per ShadowForge L1 Master Specification section 14.2.
// This file must stay stable. Dialect extensions (container, network, resilience,
// update_trait, vote, shard/async_stagger) live in ShadowRustExt.g4 and are merged
// in by the parser build (see grammar/README.md) rather than by editing this file.
grammar ShadowRust;

program: statement* EOF;

statement
    : ifStatement
    | txStatement
    | mintStatement
    | validateStatement
    | queueStatement
    | bankStatement
    | assignment
    ;

ifStatement: IF condition '{' statement* '}';
txStatement: TX BUY ID FROM ID TO ID AMOUNT expr '{' statement* '}';
mintStatement: MINT ID AMOUNT expr (EPOCH expr)? ';';
validateStatement: VALIDATE ID STAGE NUMBER '{' statement* '}';
queueStatement: QUEUE INSERT ID POSITIONS expr (',' expr)* ';';
bankStatement: BANK DEPOSIT ID ATR expr ';';
assignment: ID '=' expr ';';

condition: expr;

expr: relExpr (TO ID)?;
relExpr: arithExpr (op=('>='|'>'|'<='|'<'|'=='|'!=') arithExpr)*;
arithExpr: term (op=('+'|'-') term)*;
term: factor (op=('*'|'/') factor)*;
factor: ID | NUMBER | '(' expr ')';

TX:'tx'; BUY:'buy'; FROM:'from'; TO:'to'; AMOUNT:'amount';
IF:'if'; MINT:'mint'; EPOCH:'epoch'; VALIDATE:'validate';
STAGE:'stage'; QUEUE:'queue'; INSERT:'insert'; POSITIONS:'positions';
BANK:'bank'; DEPOSIT:'deposit'; ATR:'atr';

ID: [a-zA-Z_][a-zA-Z0-9_]*;
NUMBER: [0-9]+ ('.' [0-9]+)?;
WS: [ \t\r\n]+ -> skip;
COMMENT: '//' ~[\r\n]* -> skip;
