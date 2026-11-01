# kiwi-star-deployer

Go CLI that releases the kiwiproject libraries to Maven Central in dependency
order.

## Documentation authority

- README.md is the authoritative description of current behavior.
- DESIGN.md is a historical artifact preserved as originally written. Never use
  it as a source of truth for how the tool works or should work; where it
  disagrees with the code or README, it is simply outdated.

## Checks

Run `make check` (go vet, tests with -race, golangci-lint) before committing.

## Console colors

The release console is colored by matching the text of the messages it prints,
not by the code that prints them. If you change or add a message, read the
package comment in internal/color, which says what to check.
