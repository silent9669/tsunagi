# tsunagi

A local router that lets one Claude Code session use models from many providers, chosen with `/model`.

## Language

**Router**:
The local process Claude Code talks to. It picks a Provider per request and forwards it.
_Avoid_: proxy, gateway, relay

**Session**:
One launched Claude Code process together with the Router instance serving it.
_Avoid_: run, instance

**Provider**:
A service that serves models and is paid for with the user's own Key.
_Avoid_: backend, vendor, upstream

**Claude provider**:
Anthropic itself, reached with the user's own Claude Code login. The Router never holds its credential.
_Avoid_: Claude branch, default provider

**Key**:
A static credential the user pastes in for one Provider. Never a Claude login.
_Avoid_: token, secret, API key (when ambiguous)

**Sign-in**:
Logging in to a Provider with the user's own subscription through OAuth, instead of a Key.
_Avoid_: OAuth login, subscription auth

**Route**:
The rule that maps a model name to one Provider.
_Avoid_: mapping, rule

**Dialect**:
The request format a Provider speaks: `anthropic`, `openai-chat` or `openai-responses`.
_Avoid_: protocol, format, API type

**Translator**:
The part that converts a request and its streamed reply between the `anthropic` Dialect and another Dialect.
_Avoid_: adapter, shim

**Hand-off**:
Sending a finished plan from a normal Claude session into a tsunagi Session and getting a completion message back.
_Avoid_: delegation, relay
