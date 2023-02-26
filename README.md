# DiffStream: dynamic multi-channel text diff library
DiffStream is designed to process multiple streams of text into a unified text diff structure. Take these three text samples:

0) The pen is red.
1) The pen is not red.
2) The pen is blue.

When written to the DiffStream, they yield the following text chunks:

| 2 1 0 | value |
| ----- | ----- |
| 1 1 1 | The Pen is |
| 0 1 0 | not |
| 0 1 1 | red |
| 1 0 0 | blue |

Text chunks include a bit mask indicating in which stream the chunk appears.