program arrays(output);
type
  index = 1..10;
  row = array [index] of integer;
  matrix = array [1..10, 1..10] of integer;
  name = packed array [1..8] of char;
var
  r: row;
  m: matrix;
  s: name;
  grid: array [1..2] of array [1..3] of integer;
  n: integer;
begin
  r[1] := 1;
  n := r[1];
  r[n] := 1;
  m[1, n] := r[n];
  n := grid[1][2];
  s[1] := 'a'
end.
