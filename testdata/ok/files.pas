program files(output);
type
  numfile = file of integer;
  textpack = packed file of char;
var
  f: numfile;
  g: file of array [1..2] of integer;
begin
  f^ := 1;
  g^[1] := f^
end.
