"""Portable tests of install preflight; never run the installer."""
import importlib.util,pathlib,struct,tempfile,unittest
root=pathlib.Path(__file__).resolve().parent.parent
spec=importlib.util.spec_from_file_location('preflight',root/'deploy/nas/check-platform.py');m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
class PreflightTests(unittest.TestCase):
 def check(self,machine,arch,dynamic=False):
  header=bytearray(64);header[:6]=b'\x7fELF\x02\x01';struct.pack_into('<HH',header,16,2,machine);struct.pack_into('<Q',header,32,64);struct.pack_into('<HH',header,54,56,1)
  ph=bytearray(56);struct.pack_into('<I',ph,0,3 if dynamic else 1)
  with tempfile.TemporaryDirectory() as folder:
   p=pathlib.Path(folder)/'binary';p.write_bytes(header+ph);m.check_binary(p,arch)
 def test_supported_architectures(self):
  self.assertEqual(m.architecture('aarch64'),'arm64');self.assertEqual(m.architecture('x86_64'),'amd64')
  with self.assertRaises(ValueError):m.architecture('armv7l')
 def test_matching_static(self):self.check(183,'arm64');self.check(62,'amd64')
 def test_wrong_machine(self):
  with self.assertRaises(ValueError):self.check(62,'arm64')
  with self.assertRaises(ValueError):self.check(183,'amd64')
 def test_dynamic_binary_refused(self):
  with self.assertRaises(ValueError):self.check(183,'arm64',True)
 def test_non_elf_refused(self):
  with tempfile.TemporaryDirectory() as folder:
   p=pathlib.Path(folder)/'binary';p.write_bytes(b'MZ'+b'0'*100)
   with self.assertRaises(ValueError):m.check_binary(p,'arm64')
if __name__=='__main__':unittest.main()
