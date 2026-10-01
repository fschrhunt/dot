# Contributing

Smallness is the point. dot is one file with no dependencies beyond the Python 3.11+ standard
library and git. A change that adds a file, a dependency or an option has to earn it; a change that
removes one is welcome.

## Setup and tests

```sh
git clone https://github.com/fschrhunt/dot
cd dot
python3 test.py
```

Tests run offline in temp folders. Add one focused test per behavior you change.

## Issues

Include the output of `dot help`, which shows your machine name, values and mappings. Remove
anything private first.

## License

By contributing, you agree that your contributions are licensed under the MIT License.
