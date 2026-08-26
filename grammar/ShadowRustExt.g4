// ShadowRust dialect extensions, section 14.3. Composed via ANTLR grammar import so
// grammar/ShadowRust.g4 (the core file Phase 1 must parse, per spec section 14.2)
// stays untouched. This is the grammar the production parser is generated from.
grammar ShadowRustExt;
import ShadowRust;

statement
    : ifStatement
    | txStatement
    | mintStatement
    | validateStatement
    | queueStatement
    | bankStatement
    | containerStatement
    | networkStatement
    | resilienceStatement
    | activateStatement
    | updateTraitStatement
    | voteStatement
    | shardStatement
    | asyncStaggerStatement
    | assignment
    ;

// container { id=...; validators=...; hybrid=50; sync_tps=...; interval=...; }
containerStatement: CONTAINER '{' containerField* '}';
containerField: ID '=' expr ';';

// network { listen=...; bootstrap=...; }
networkStatement: NETWORK '{' networkField* '}';
networkField: ID '=' expr ';';

// resilience if online < 10 { activate sentinels; }
resilienceStatement: RESILIENCE IF condition '{' statement* '}';
activateStatement: ACTIVATE SENTINELS ';';

// update_trait ID KEY op expr ;
updateTraitStatement: UPDATE_TRAIT ID ID op=('='|'+='|'-=') expr ';';

// vote PROPOSAL commitment ;
voteStatement: VOTE ID ID ';';

// shard dialect
shardStatement: SHARD ID (COUNT expr)? ';';

// async_stagger dialect
asyncStaggerStatement: ASYNC_STAGGER '{' staggerField* '}';
staggerField: ID '=' expr ';';

// Overrides core factor (14.2) to let a tx body refer back to its own
// `amount` value, as the canonical example in spec section 14.5 does
// (`project_fee = amount * 0.05 to vault_address;`). AMOUNT is otherwise a
// reserved keyword introducing the tx header's amount clause; this add-only
// alternative resolves that spec-internal inconsistency per the Phase 1
// "internal review week" directive (spec 18.3) without touching the pinned
// core grammar file.
factor: ID | AMOUNT | NUMBER | '(' expr ')';

CONTAINER: 'container';
NETWORK: 'network';
RESILIENCE: 'resilience';
ACTIVATE: 'activate';
SENTINELS: 'sentinels';
UPDATE_TRAIT: 'update_trait';
VOTE: 'vote';
SHARD: 'shard';
ASYNC_STAGGER: 'async_stagger';
COUNT: 'count';
PLUSEQ: '+=';
MINUSEQ: '-=';
