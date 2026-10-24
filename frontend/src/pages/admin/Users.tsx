import { useCallback, useState } from 'react'
import type { FormEvent } from 'react'
import { read, write } from '../../api/admin'
import type { User } from '../../api/admin'
import { useAdmin, panel } from '../../components/admin/useAdmin'
export default function Users(){
 const load=useCallback(()=>read<User[]>('/admin/users'),[]),s=useAdmin(load)
 const [id,setId]=useState<number|null>(null),[username,setUsername]=useState(''),[password,setPassword]=useState('')
 function reset(){setId(null);setUsername('');setPassword('')}
 async function save(e:FormEvent){e.preventDefault();const body=id===null?{username,password}:{username,...(password?{password}:{})};if(await s.act(()=>write(id===null?'POST':'PATCH',id===null?'/admin/users':`/admin/users/${id}`,body)))reset()}
 return <section><h1>Users</h1>{s.error&&<p role="alert">{s.error}</p>}<form onSubmit={save} style={panel}><h2>{id===null?'Buat user':'Edit user'}</h2><label>Username <input required maxLength={64} value={username} onChange={e=>setUsername(e.target.value)}/></label> <label>{id===null?'Password':'Password baru (kosong = tetap)'} <input type="password" autoComplete="new-password" required={id===null} minLength={8} value={password} onChange={e=>setPassword(e.target.value)}/></label> <button disabled={s.busy}>Simpan</button> <button type="button" onClick={reset}>Batal</button></form>
 {s.data===null?<p>Memuat...</p>:<table><thead><tr><th>Username</th><th>Role</th><th>Aksi</th></tr></thead><tbody>{s.data.map(u=><tr key={u.id}><td>{u.username}</td><td>{u.isAdmin?'Platform Admin':'User'}</td><td><button disabled={s.busy} onClick={()=>{setId(u.id);setUsername(u.username);setPassword('')}}>Edit</button> <button disabled={s.busy} onClick={()=>{if(confirm(`${u.isAdmin?'Cabut':'Beri'} admin untuk ${u.username}?`))void s.act(()=>write(u.isAdmin?'DELETE':'PUT',`/admin/users/${u.id}/roles/platform_admin`))}}>{u.isAdmin?'Cabut admin':'Beri admin'}</button> <button disabled={s.busy} onClick={()=>{if(confirm(`Hapus ${u.username}? Semua session user akan dicabut.`))void s.act(()=>write('DELETE',`/admin/users/${u.id}`))}}>Hapus</button></td></tr>)}</tbody></table>}</section>
}
