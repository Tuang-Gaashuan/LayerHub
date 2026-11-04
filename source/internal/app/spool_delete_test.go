package app
import "testing"
func TestDeleteUnusedSpool(t *testing.T){
 s,u,d:=setup(t);_,sp:=fixture(t,s,u)
 if err:=s.DeleteSpool(u,sp.ID,sp.Revision);err!=nil{t.Fatal(err)}
 rows,_:=readSpools(s.DB);if len(rows)!=0{t.Fatal("record remains")}
 if d.uploads!=0{t.Fatal("physical transfer")}
 if s.DeleteSpool(u,sp.ID,sp.Revision)==nil{t.Fatal("deleted twice")}
}
func TestDeleteSpoolProtectsHistoryMountAndRevision(t *testing.T){
 for _,kind:=range []string{"queued","done","mapping","mounted","revision"}{t.Run(kind,func(t *testing.T){
  s,u,_:=setup(t);p,sp:=fixture(t,s,u)
  switch kind{
  case "queued","done","mapping":
   id:=newJob(t,s,u,p,sp,10)
   if kind=="done"{s.DB.Exec("UPDATE jobs SET state='done' WHERE id=?",id)}
   if kind=="mapping"{s.DB.Exec(`UPDATE jobs SET print_options=? WHERE id=?`, `{"mapping":[{"spoolId":"other","grams":10}]}`,id)}
  case "mounted":s.DB.Exec("UPDATE spools SET mount_printer=?,mount_slot='1' WHERE id=?",p.ID,sp.ID)
  case "revision":sp.Revision++
  }
  if s.DeleteSpool(u,sp.ID,sp.Revision)==nil{t.Fatal("protected record deleted")}
  var count int;s.DB.QueryRow("SELECT COUNT(*) FROM spools WHERE id=?",sp.ID).Scan(&count);if count!=1{t.Fatal("record changed")}
 })}
}
func TestDeleteSpoolPermissions(t *testing.T){
 s,u,_:=setup(t);_,sp:=fixture(t,s,u)
 if err:=s.SaveUser(u,"member","Member","TestPassword123",false);err!=nil{t.Fatal(err)}
 var id int64;s.DB.QueryRow("SELECT id FROM users WHERE username='member'").Scan(&id)
 if s.DeleteSpool(User{ID:id,Admin:true},sp.ID,sp.Revision)==nil{t.Fatal("forged privilege accepted")}
 s.DB.Exec("UPDATE spools SET owner_id=? WHERE id=?",id,sp.ID)
 if s.DeleteSpool(u,sp.ID,sp.Revision)==nil{t.Fatal("admin deleted personal record")}
 if err:=s.DeleteSpool(User{ID:id},sp.ID,sp.Revision);err!=nil{t.Fatal(err)}
}
