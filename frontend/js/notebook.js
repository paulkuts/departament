import {api} from './api.js';
import {el,button,link,external,field,form,modal,closeModal,confirmAction,toast,date,status,table,actions,sheet,details} from './dom.js';

const content = document.getElementById('content');
let me = null, revision = 0, blobURLs = [];
const admin = () => me?.role === 'admin';
const roles = {admin:'Администратор',staff:'Сотрудник',teacher:'Преподаватель',student:'Студент'};
const types = {equipment:'Оборудование',inventory:'Мебель и инвентарь',raw_material:'Химикаты и материалы',other:'Посуда и другое'};
const keyStatuses = {available:'Свободен',issued:'Выдан',lost:'Утерян'};
const articleStatuses = {planned:'В работе',submitted:'На рассмотрении',published:'Опубликована'};
const labels = {overview:'Рабочий обзор',equipment:'Оборудование',inventory:'Инвентаризация',keys:'Ключи',events:'Задачи и события',articles:'Публикации',reference:'Справочник',assistants:'Ассистенты',notes:'Мои заметки',profile:'Мой профиль',users:'Коллеги и доступ',requests:'Заявки на ключи',welcome:'О кафедре',login:'Вход',register:'Регистрация'};
const request = (path, method = 'GET', body) => api.request(path, {method, ...(body ? {body:JSON.stringify(body)} : {})});
const navTo = (path) => { if (location.hash === `#/${path}`) route(); else location.hash = `/${path}`; };
const head = (title, subtitle, ...buttons) => el('div',{class:'page-heading'},el('div',{},el('h1',{},title),el('p',{},subtitle)),actions(buttons));
const sh = (title, hint, ...buttons) => el('div',{class:'sheet-head'},el('div',{},el('h2',{},title),hint ? el('p',{},hint) : null),actions(buttons));
const body = (...nodes) => el('div',{class:'sheet-body'},nodes);
const choose = (obj) => Object.entries(obj);
const nullable = (data) => Object.fromEntries(Object.entries(data).map(([k,v]) => [k,typeof v === 'string' ? v.trim() || null : v]));
const initials = name => (name || '').split(/\s+/).slice(0,2).map(x => x[0] || '').join('');
const input = (name,label,value,opts={}) => field(name,label,value,opts);
const select = (name,label,value,choices) => field(name,label,value,{choices});
const remove = (title, name, fn) => confirmAction(title,`«${name}» будет удалено. Это действие нельзя отменить.`, async () => {await fn(); toast('Запись удалена'); route();});
function paint(node, token) { if (token === revision) content.replaceChildren(node); }
function shell(page) {
  const rail = document.getElementById('rail');
  const groups = me ? [
    ['', [['overview','Обзор','▦']]],
    ['Реестры',[['equipment','Оборудование','⌕'],['inventory','Инвентаризация','▤'],['keys','Ключи','⚿'],['events','Задачи и события','◷']]],
    ['Научная работа',[['articles','Публикации','≡'],['reference','Справочник','▥'],['assistants','Ассистенты','◇']]],
    ['Личное',[['notes','Мои заметки','▧'],['profile','Мой профиль','○']]],
    ...(admin() ? [['Управление',[['users','Коллеги и доступ','♧'],['requests','Заявки на ключи','↗']]]] : [])
  ] : [['',[['welcome','О пространстве','▦'],['login','Войти','→'],['register','Регистрация','＋']]]];
  rail.replaceChildren(link('','#/overview','brand'));
  rail.firstChild.append(el('span',{class:'brand-mark','aria-hidden':'true'},'▣'),el('strong',{},'Контур кафедры'),el('small',{},'Лабораторный журнал'));
  const nav = el('nav',{class:'nav','aria-label':'Разделы'});
  for (const [group,items] of groups) {
    if (group) nav.append(el('p',{class:'nav-label'},group));
    for (const [key,title,icon] of items) nav.append(el('a',{href:`#/${key}`,'aria-current':page===key?'page':null,onclick:()=>rail.classList.remove('open')},el('span',{class:'nav-icon','aria-hidden':'true'},icon),title));
  }
  rail.append(nav,el('div',{class:'rail-bottom'},el('strong',{},me ? roles[me.role] || me.role : 'Гостевой доступ'),me ? 'Личное рабочее пространство' : 'Внутренние реестры доступны после входа'));
  const user = me ? actions(el('span',{class:'avatar','aria-hidden':'true'},initials(me.full_name)),link(me.full_name,'#/profile'),el('small',{},roles[me.role]),button('Выйти', async()=>{await api.logout(); me=null; navTo('welcome');})) : actions(link('Войти','#/login','btn'),link('Создать аккаунт','#/register','btn primary'));
  document.getElementById('topbar').replaceChildren(button('☰ Меню',()=>{rail.classList.toggle('open');},'mobile-menu'),el('div',{class:'crumb'},el('strong',{},'Рабочее пространство'),` / ${labels[page] || 'Карточка записи'}`),el('div',{class:'account'},user));
}
function queryRoute() {
  if (!location.hash && location.pathname.startsWith('/public/keys/')) return ['public-key',location.pathname.split('/').pop(),new URLSearchParams()];
  const [path,qs=''] = (location.hash.slice(2) || (me ? 'overview':'welcome')).split('?');
  const parts = path.split('/');
  const page = parts[0] === 'article' ? 'articles' : parts[0];
  const id = parts[1] === 'view' ? parts[2] : parts[1];
  return [page,id,new URLSearchParams(qs)];
}
async function route() {
  const token = ++revision;
  blobURLs.forEach(url=>URL.revokeObjectURL(url)); blobURLs=[];
  let [page,id,q] = queryRoute();
  if (!me && !['welcome','login','register','public-key'].includes(page)) page='login';
  if (me && ['login','register','welcome'].includes(page)) page='overview';
  shell(page); document.title = `${labels[page] || 'Карточка'} · Контур кафедры`;
  content.replaceChildren(el('p',{class:'loading',role:'status'},'Загружаем записи…'));
  try {
    let node;
    if (['welcome','login','register'].includes(page)) node=authPage(page);
    else if (page==='overview') node=await overview();
    else if (page==='equipment'||page==='inventory') node=id ? await inventoryDetail(id,page) : await inventoryList(page,q);
    else if (page==='keys') node=id ? await keyDetail(id) : await keyList(q);
    else if (page==='articles') node=id ? await articleDetail(id) : await articleList(q);
    else if (page==='events') node=await events(q);
    else if (page==='profile') node=await profile(me);
    else if (page==='users') node=admin() ? (id ? await profile(await api.getUser(id)) : await users(q)) : forbidden();
    else if (page==='notes') node=await notes(id);
    else if (page==='reference') node=await reference(q);
    else if (page==='assistants') node=assistants();
    else if (page==='requests') node=admin() ? await requestsPage() : forbidden();
    else if (page==='public-key') node=await publicKey(id);
    else node=el('div',{},head('Страница не найдена','Проверьте адрес или вернитесь к обзору.'),link('Открыть обзор','#/overview','btn'));
    paint(node,token);
  } catch(error) {
    paint(sheet(body(el('h2',{},'Не удалось загрузить страницу'),el('p',{role:'alert'},error.message),button('Повторить',route))),token);
  }
}
const forbidden = () => sheet(body(el('h2',{},'Недостаточно прав'),el('p',{},'Этот раздел доступен администраторам.'),link('Вернуться к обзору','#/overview','btn')));
function authPage(mode) {
  const register = mode==='register';
  const story = el('div',{class:'auth-story'},el('div',{class:'eyebrow'},el('strong',{},'Кафедра · рабочее пространство')),el('h1',{},'Всё, что нужно для работы кафедры.'),el('p',{},'Оборудование, ключи, научные публикации и повседневные задачи — в одном лабораторном журнале.'),el('ul',{class:'journal-list'},
    el('li',{},el('strong',{},'Вещи на своих местах'),el('p',{},'Реестры имущества, ответственные, поверки и история выдачи.')),
    el('li',{},el('strong',{},'Знания под рукой'),el('p',{},'Публикации, справочник и личные исследовательские заметки.')),
    el('li',{},el('strong',{},'Каждому — свой доступ'),el('p',{},'Сотрудники работают с данными. Администраторы управляют имуществом и правами.'))));
  const authForm = form([
    ...(register ? [input('full_name','Имя и фамилия','',{required:true,minlength:3,autocomplete:'name'})] : []),
    input('email','Электронная почта','',{type:'email',required:true,autocomplete:'username'}),
    input('password','Пароль','',{type:'password',required:true,minlength:register?8:1,autocomplete:register?'new-password':'current-password'}),
    ...(register ? [el('p',{class:'form-hint'},'Не менее 8 символов. После регистрации вы получите роль сотрудника. Изменить права сможет администратор.')] : [])
  ],register?'Создать аккаунт':'Войти',async data=>{
    if (register) await request('/auth/register','POST',data);
    await api.signIn(data.email.trim(),data.password);
    me=await api.getMe(); navTo('overview');
  });
  return el('div',{class:'auth-layout'},story,el('section',{class:'auth-form'},el('nav',{class:'auth-switch','aria-label':'Вход или регистрация'},el('a',{href:'#/login','aria-current':!register?'page':null},'Вход'),el('a',{href:'#/register','aria-current':register?'page':null},'Регистрация')),el('h2',{},register?'Присоединиться к кафедре':'Добро пожаловать'),authForm,el('p',{class:'auth-note'},'Чтобы запросить ключ без аккаунта, отсканируйте QR-код на ключе. Внутренние реестры гостям недоступны.')));
}
function filterBar(page,q,fields) {
  const box=el('form',{class:'filters',onsubmit:e=>{e.preventDefault();const params=new URLSearchParams(new FormData(box));for(const [k,v] of [...params])if(!v)params.delete(k);navTo(`${page}?${params}`);}},fields,el('button',{class:'btn',type:'submit'},'Найти'),link('Сбросить',`#/${page}`,'btn quiet'));
  return box;
}
function pager(page,q,meta={}) {
  const current=Number(meta.page)||1,total=Number(meta.total_pages)||1;
  const go=offset=>{const next=new URLSearchParams(q);next.set('offset',offset);navTo(`${page}?${next}`);};
  const prev=button('← Назад',()=>go(Math.max(0,(current-2)*20)));prev.disabled=current<=1;
  const next=button('Далее →',()=>go(current*20));next.disabled=current>=total;
  return el('div',{class:'sheet-foot'},el('span',{},`${meta.total ?? '—'} записей · страница ${current} из ${total}`),actions(prev,next));
}
async function overview() {
  const results=await Promise.allSettled([api.getInventory({limit:5}),api.getKeys(),api.getArticles({limit:3}),api.getEvents({limit:4}),api.getExpiredVerification(1,0)]);
  const values=results.map(r=>r.status==='fulfilled'?r.value:null);
  const [inv,keys,articles,ev,expired]=values;
  const root=el('div',{},el('div',{class:'eyebrow'},el('strong',{},'Лабораторный журнал'),el('span',{},new Date().toLocaleDateString('ru-RU',{day:'numeric',month:'long',weekday:'long'}))),head('Рабочий обзор','Состояние реестров и ближайшие дела. Всё, что требует внимания сегодня.',...(admin()?[button('Добавить объект',()=>inventoryForm(null,'equipment'),'primary')]:[link('Мои заметки','#/notes','btn primary')]),link('Открыть ключи','#/keys','btn')));
  const failures=results.filter(r=>r.status==='rejected').length;
  if(failures)root.append(el('div',{class:'notice',role:'alert'},el('div',{},el('strong',{},'Часть сводки недоступна'),el('p',{},'Сервер не вернул все данные. Остальные реестры доступны.')),button('Повторить',route)));
  const count=expired?.paginated_metadata?.total || 0;
  if(count)root.append(el('div',{class:'notice'},el('div',{},el('strong',{},`${count} объектов с просроченной поверкой`),el('p',{},'Проверьте сроки и назначьте обслуживание.')),link('Посмотреть объекты','#/equipment?expired=1')));
  const items=inv?.inventory || [];
  const main=sheet(sh('Имущество под наблюдением','Последние записи реестра',link('Все объекты','#/inventory','btn quiet')),table(['Объект','Инв. №','Расположение','Состояние'],items.map(x=>[link(x.name,`#/equipment/${x.id}`,'record-link'),x.inventory_number,x.location,status(x.status?'Доступно':'Недоступно',x.status?'good':'warn')])),el('div',{class:'sheet-foot'},el('span',{},`${inv?.paginated_metadata?.total ?? '—'} объектов в реестре`),link('Проверить инвентаризацию','#/inventory')));
  const tasks=sheet(sh('Ближайшие задачи и события','План работы кафедры',link('Все','#/events','btn quiet')),ev?.events?.length ? el('ul',{class:'journal-list'},ev.events.map(x=>el('li',{},button(x.title,()=>eventView(x),'quiet'),el('small',{},`${date(x.start_time,true)} · ${x.location || 'Место не указано'}`)))):body(el('p',{},'Предстоящих событий пока нет.')));
  const side=el('div',{class:'overview-side'},sheet(sh('Публикации','Научная работа',link('Все','#/articles','btn quiet')),articles?.articles?.length?el('ul',{class:'journal-list'},articles.articles.map(x=>el('li',{},link(x.title,`#/articles/${x.id}`),el('small',{},articleStatuses[x.status]||x.status)))):body(el('p',{},'Публикации ещё не добавлены.'))),sheet(sh('Ключи','Текущий статус'),el('div',{class:'summary-line'},el('div',{},el('strong',{},keys?.filter(k=>k.status==='available').length ?? '—'),el('span',{},'свободно')),el('div',{},el('strong',{},keys?.filter(k=>k.status==='issued').length ?? '—'),el('span',{},'выдано'))),body(link('Открыть журнал ключей','#/keys'))));
  root.append(el('div',{class:'overview-grid'},el('div',{},main,tasks),side));return root;
}
async function inventoryList(page,q) {
  const params={limit:20,offset:Number(q.get('offset'))||0,search:q.get('search'),inventory:q.get('inventory'),type:q.get('type') || (page==='equipment'?'equipment':''),status:q.get('status')};
  const data=q.get('expired') ? await api.getExpiredVerification(20,params.offset) : await api.getInventory(params);
  const tabs=el('nav',{class:'register-tabs','aria-label':'Категории имущества'},[['','Все имущество'],...choose(types)].map(([value,title])=>el('a',{href:`#/inventory${value?'?type='+value:''}`,'aria-current':(q.get('type')||'')===value?'page':null},title)));
  return el('div',{},head(labels[page],page==='equipment'?'Приборы, доступность, поверки и передача во временное пользование.':'Мебель, химикаты, лабораторная посуда и другое имущество кафедры.',...(admin()?[button('Добавить объект',()=>inventoryForm(null,page),'primary')]:[])),page==='inventory'?tabs:null,sheet(
    filterBar(page,q,[input('search','Название',q.get('search')||'',{placeholder:'Найти объект…'}),input('inventory','Инвентарный номер',q.get('inventory')||''),select('type','Категория',params.type,[['','Все категории'],...choose(types)]),select('status','Доступность',q.get('status')||'',[['','Любая'],['available','Доступно'],['unavailable','Недоступно']])]),
    q.get('expired')?body(status('Показаны просроченные поверки','warn'),link('Снять фильтр',`#/${page}`)):null,
    table(['Объект','Категория','Инв. №','Расположение','Следующая поверка','Состояние'],(data.inventory||[]).map(x=>[link(x.name,`#/${page}/${x.id}`,'record-link'),types[x.type]||x.type,x.inventory_number,x.location,verification(x.next_verification_date),status(x.status?'Доступно':x.unavailable_reason||'Недоступно',x.status?'good':'warn')])),pager(page,q,data.paginated_metadata)),link('Показать просроченные поверки',`#/${page}?expired=1`,'btn quiet'));
}
function verification(value) {return value ? status(date(value),new Date(value)<new Date()?'warn':'good') : '—';}
async function inventoryForm(item,page='equipment') {
  const x=item||{status:true,type:page==='equipment'?'equipment':'inventory'};
  const people=await api.getActiveUsers();
  modal(item?'Редактировать объект':'Новый объект',form([
    input('name','Название',x.name,{required:true}),select('type','Категория',x.type,choose(types)),input('description','Описание и особенности',x.description,{type:'textarea'}),
    el('div',{class:'form-grid'},input('inventory_number','Инвентарный номер',x.inventory_number),input('location','Расположение',x.location,{required:true})),
    select('responsible_id','Ответственный',x.responsible_id||'',[['','Выберите сотрудника'],...people.map(p=>[p.id,p.full_name])]),input('documentation','Документация (ссылка или текст)',x.documentation),
    el('div',{class:'form-grid'},input('last_verification_date','Последняя поверка',x.last_verification_date?.slice(0,10),{type:'date'}),input('next_verification_date','Следующая поверка',x.next_verification_date?.slice(0,10),{type:'date'})),
    input('status','Доступно для использования',x.status,{type:'checkbox'}),input('unavailable_reason','Причина недоступности',x.unavailable_reason)
  ],'Сохранить',async data=>{if(!data.responsible_id)throw Error('Выберите ответственного сотрудника');const payload={...nullable(data),status:data.status==='on',unavailable_reason:data.status==='on'?null:data.unavailable_reason||null};if(item)await api.updateInventory(x.id,payload);else await api.createInventory(payload);closeModal();toast('Объект сохранён');route();}));
}
async function blobImage(url,alt,className='') {
  const res=await fetch(url,{headers:{Authorization:`Bearer ${api.accessToken}`},credentials:'same-origin'});
  if(!res.ok)return el('span',{class:'muted'},'Изображение недоступно');
  const src=URL.createObjectURL(await res.blob());blobURLs.push(src);return el('img',{src,alt,class:className});
}
async function inventoryDetail(id,page) {
  const x=await api.getInventoryById(id);
  const [photos,comments,loans]=await Promise.all([api.getPhotos(id),request(`/inventory/${id}/comments`),request(`/inventory/${id}/loans`)]);
  let responsible='Не назначен';if(x.responsible_id){try{responsible=(await api.getUser(x.responsible_id)).full_name;}catch{responsible='Недоступен';}}
  const activeLoan=loans.find(l=>!l.returned_at);
  const root=el('div',{},link('← К реестру',`#/${page}`,'back-link'),head(x.name,`${types[x.type]||x.type} · ${x.inventory_number||'Без инвентарного номера'}`,admin()?button('Редактировать',()=>inventoryForm(x,page),'primary'):null),el('div',{class:'detail-grid'},sheet(sh('Карточка объекта'),body(details([['Расположение',x.location],['Ответственный',responsible],['Состояние',status(x.status?'Доступно':x.unavailable_reason||'Недоступно',x.status?'good':'warn')],['Следующая поверка',verification(x.next_verification_date)],['Последняя поверка',date(x.last_verification_date)],['Документация',x.documentation?external(x.documentation,x.documentation):'—']])),body(el('h3',{},'Описание'),el('p',{class:'prose'},x.description||'Описание пока не добавлено.'))),sheet(sh('Временное пользование',activeLoan?'Объект сейчас выдан':'Нет активной выдачи'),body(activeLoan?details([['Получатель',activeLoan.borrower],['Дата выдачи',date(activeLoan.issued_at,true)],['Комментарий',activeLoan.comment]]):el('p',{},'Передайте оборудование сотруднику и сохраните запись о выдаче.'),admin()&&x.type==='equipment'?button(activeLoan?'Оформить возврат':'Выдать оборудование',async()=>{if(activeLoan){confirmAction('Оформить возврат',`Подтвердите возврат «${x.name}».`,async()=>{await request(`/loans/${activeLoan.id}/return`,'POST',{});route();});}else await loanForm(x);},'primary'):null))));
  const gallery=el('div',{class:'photo-grid'});
  for(const p of photos||[])gallery.append(el('figure',{},await blobImage(api.photoUrl(p.id),p.filename||x.name),el('figcaption',{},p.filename),admin()?button('Удалить фото',()=>remove('Удалить фото',p.filename||x.name,()=>api.deletePhoto(p.id)),'quiet'):null));
  root.append(sheet(sh('Фотографии и QR'),body(gallery,admin()?upload('Добавить фотографию',file=>api.uploadPhoto(id,file)):null,button('Показать QR объекта',async()=>modal('QR объекта',el('div',{},await blobImage(api.qrCodeUrl(id),'QR объекта','qr'),el('p',{},'Карточка доступна после входа.')))))));
  root.append(sheet(sh('Комментарии',`${comments.length} записей`),el('ul',{class:'journal-list'},comments.map(c=>el('li',{},el('strong',{},c.author||'Сотрудник'),el('small',{},date(c.created_at,true)),el('p',{class:'prose'},c.body)))),body(form([input('body','Добавить комментарий','',{type:'textarea',required:true,maxlength:5000})],'Отправить',async data=>{await request(`/inventory/${id}/comments`,'POST',data);route();}))));
  if(loans.length)root.append(sheet(sh('История выдачи'),table(['Получатель','Выдано','Возвращено','Комментарий'],loans.map(l=>[l.borrower,date(l.issued_at,true),l.returned_at?date(l.returned_at,true):status('На руках','warn'),l.comment]))));
  if(admin())root.append(button('Удалить объект',()=>remove('Удалить объект',x.name,()=>api.deleteInventory(id)),'danger'));return root;
}
function upload(label,save) {return el('label',{class:'field'},el('span',{},label),el('input',{type:'file',accept:'image/jpeg,image/png,image/webp',onchange:async e=>{const node=e.currentTarget,file=node.files[0];if(!file)return;if(file.size>10*1024*1024){toast('Максимальный размер — 10 МБ');return;}node.disabled=true;try{await save(file);toast('Файл загружен');route();}catch(err){toast(err.message);}finally{node.disabled=false;node.value='';}}}));}
async function loanForm(x) {
  const people=await api.getActiveUsers();modal(`Выдать: ${x.name}`,form([select('borrower_id','Получатель',people[0]?.id||'',people.map(p=>[p.id,p.full_name])),input('comment','Комментарий','',{type:'textarea'})],'Оформить выдачу',async data=>{await request(`/inventory/${x.id}/loans`,'POST',data);closeModal();route();}));
}
async function keyList(q) {
  const data=await api.getKeys(q.get('status')||'');
  const search=(q.get('search')||'').toLowerCase();
  const filtered=data.filter(k=>`${k.key_number} ${k.room_description}`.toLowerCase().includes(search));
  return el('div',{},head('Ключи','Выдача, возврат и история. Карточки ключей создаёт и удаляет только администратор.',admin()?button('Добавить ключ',()=>keyForm(),'primary'):null),sheet(filterBar('keys',q,[input('search','Поиск',q.get('search')||'',{placeholder:'Номер или помещение'}),select('status','Состояние',q.get('status')||'',[['','Все состояния'],...choose(keyStatuses)])]),table(['Номер ключа','Помещение','Статус',''],filtered.map(k=>[link(k.key_number,`#/keys/${k.id}`,'record-link'),k.room_description,status(keyStatuses[k.status]||k.status,k.status==='available'?'good':'warn'),link('Открыть журнал',`#/keys/${k.id}`)])),el('div',{class:'sheet-foot'},`${filtered.length} ключей в выборке`)));
}
async function keyDetail(id) {
  const [x,history]=await Promise.all([api.getKey(id),api.getKeyHistory(id)]);
  const holder=x.status==='issued'?await api.getKeyHolder(id):null;
  const holderID=holder?.user_id;
  let holderName='—';if(holderID){try{holderName=(await api.getUser(holderID)).full_name;}catch{holderName='Получатель недоступен';}}
  const operations=actions();
  if(x.status==='available')operations.append(button(admin()?'Выдать ключ':'Взять ключ',()=>issueKey(x),'primary'));
  if(x.status==='issued'&&(admin()||holderID===me.id))operations.append(button('Вернуть ключ',()=>modal(`Возврат ключа ${x.key_number}`,form([input('comment','Комментарий','',{type:'textarea'})],'Подтвердить возврат',async data=>{await api.returnKey(id,data);closeModal();route();})), 'primary'));
  if(admin())operations.append(button('Редактировать',()=>keyForm(x)),button('QR для гостей',async()=>{const data=await request(`/keys/${id}/public-link`,'POST',{});const url=new URL(data.path,location.origin).href;modal(`QR ключа ${x.key_number}`,el('div',{},await blobImage(`/api/v1/keys/${id}/qr`,'QR для запроса ключа','qr'),el('p',{class:'break'},link(url,url)),el('p',{},'Гость откроет форму запроса. Выдачу подтверждает администратор.'),button('Распечатать',()=>window.print())));}));
  return el('div',{},link('← К реестру ключей','#/keys','back-link'),head(`Ключ ${x.key_number}`,x.room_description),sheet(sh('Карточка ключа'),body(details([['Состояние',status(keyStatuses[x.status],x.status==='available'?'good':'warn')],['Сейчас у',holderName],['Примечания',x.notes||'—']])),body(operations)),sheet(sh('Журнал операций','Все выдачи и возвраты'),table(['Действие','Дата','Комментарий'],(history||[]).map(h=>[{issue:'Выдан',return:'Возвращён',lost:'Утрачен'}[h.action_type]||h.action_type,date(h.timestamp,true),h.comment]))),admin()?actions(button('Отметить утерю',()=>confirmAction('Отметить утерю',`Ключ ${x.key_number} будет отмечен как утерянный.`,async()=>{await api.markLost(id,{comment:'Утеря отмечена администратором'});route();}),'danger'),button('Удалить ключ',()=>remove('Удалить ключ',x.key_number,()=>request(`/keys/${id}`,'DELETE')),'danger')):null);
}
function keyForm(x) {
  modal(x?'Редактировать ключ':'Новый ключ',form([input('key_number','Номер ключа',x?.key_number,{required:true}),input('room_description','Помещение',x?.room_description,{required:true}),input('notes','Примечания',x?.notes,{type:'textarea'})],'Сохранить',async data=>{if(x)await api.updateKey(x.id,nullable(data));else await api.createKey(nullable(data));closeModal();route();}));
}
async function issueKey(x) {
  const people=admin()?await api.getActiveUsers():[me];
  modal(`Выдать ключ ${x.key_number}`,form([select('user_id','Получатель',me.id,people.map(p=>[p.id,p.full_name])),input('comment','Комментарий','',{type:'textarea'})],'Подтвердить выдачу',async data=>{await api.issueKey(x.id,data);closeModal();route();}));
}
async function articleList(q) {
  const params={limit:20,offset:Number(q.get('offset'))||0,search:q.get('search'),status:q.get('status'),author_id:q.get('mine')?me.id:q.get('author_id')};
  const data=await api.getArticles(params);
  return el('div',{},head('Публикации','Рукописи, авторы, выходные данные и статусы научных работ.',button('Добавить публикацию',()=>articleForm(),'primary')),el('nav',{class:'register-tabs','aria-label':'Публикации'},el('a',{href:'#/articles','aria-current':!q.get('mine')?'page':null},'Все публикации'),el('a',{href:'#/articles?mine=1','aria-current':q.get('mine')?'page':null},'Мои работы')),sheet(filterBar('articles',q,[input('search','Название',q.get('search')||'',{placeholder:'Найти публикацию…'}),select('status','Статус',q.get('status')||'',[['','Все статусы'],...choose(articleStatuses)]),q.get('mine')?el('input',{type:'hidden',name:'mine',value:'1'}):null,q.get('author_id')?el('input',{type:'hidden',name:'author_id',value:q.get('author_id')}):null]),table(['Публикация и авторы','Индексирование','Белый список','Статус'],(data.articles||[]).map(x=>[el('div',{},link(x.title,`#/articles/${x.id}`,'record-link'),el('small',{},(x.authors||[]).map(a=>a.name).join(', '))),x.indexing,x.white_list_level,status(articleStatuses[x.status]||x.status,x.status==='published'?'good':'')])),pager('articles',q,data.paginated_metadata)));
}
async function articleDetail(id) {
  const x=await api.getArticle(id),canEdit=admin()||x.created_by===me.id;
  return el('div',{},link('← К публикациям','#/articles','back-link'),head(x.title,(x.authors||[]).map(a=>a.name).join(', '),canEdit?button('Редактировать',()=>articleForm(x),'primary'):null),sheet(sh('Паспорт публикации'),body(details([['Статус',status(articleStatuses[x.status],x.status==='published'?'good':'')],['Индексирование',x.indexing],['Белый список',x.white_list_level],['Финансирование',x.funding],['Ссылка',x.link?external('Открыть публикацию',x.link):'—'],['Обновлено',date(x.updated_at)]])),body(el('h3',{},'Выходные данные'),el('p',{class:'prose'},x.details||'Выходные данные пока не указаны.'))),canEdit?button('Удалить публикацию',()=>remove('Удалить публикацию',x.title,()=>api.deleteArticle(id)),'danger'):null);
}
async function articleForm(x) {
  const people=await api.getActiveUsers();
  const authorBox=el('div',{});
  function addAuthor(a={}) {
    const user=el('select',{'aria-label':'Сотрудник-автор'},el('option',{value:''},'Внешний автор'),people.map(p=>el('option',{value:p.id},p.full_name)));user.value=a.user_id||'';
    const name=el('input',{'aria-label':'Имя автора',value:a.name||'',required:true,placeholder:'Имя автора'});
    user.addEventListener('change',()=>{const p=people.find(p=>p.id===user.value);if(p)name.value=p.full_name;});
    const row=el('div',{class:'author-row'},user,name,button('Убрать',()=>row.remove(),'quiet'));authorBox.append(row);
  }
  (x?.authors?.length?x.authors:[{user_id:me.id,name:me.full_name}]).forEach(addAuthor);
  modal(x?'Редактировать публикацию':'Новая публикация',form([
    input('title','Название',x?.title,{required:true,type:'textarea',rows:2}),el('div',{},el('h3',{},'Авторы'),authorBox,button('Добавить автора',()=>addAuthor())),input('details','Выходные данные',x?.details,{type:'textarea'}),
    el('div',{class:'form-grid'},input('indexing','Индексирование',x?.indexing,{placeholder:'Scopus Q1, ВАК…'}),input('white_list_level','Белый список',x?.white_list_level)),input('funding','Финансирование',x?.funding),input('link','Ссылка на публикацию',x?.link,{type:'url'}),select('status','Статус',x?.status||'planned',choose(articleStatuses))
  ],'Сохранить',async data=>{const authors=[...authorBox.children].map(row=>({user_id:row.querySelector('select').value||null,name:row.querySelector('input').value.trim()}));if(!authors.length)throw Error('Добавьте хотя бы одного автора');const payload={...nullable(data),authors};if(x)await api.updateArticle(x.id,payload);else await api.createArticle(payload);closeModal();route();}));
}
async function events(q) {
  const data=await api.getEvents({limit:20,offset:Number(q.get('offset'))||0,title:q.get('title'),first_date:q.get('first_date'),last_date:q.get('last_date')});
  return el('div',{},head('Задачи и события','Уборки, инвентаризации, встречи и другие запланированные дела кафедры.',me.role!=='student'?button('Добавить событие',()=>eventForm(),'primary'):null),sheet(filterBar('events',q,[input('title','Название',q.get('title')||'',{placeholder:'Найти дело…'}),input('first_date','С даты',q.get('first_date')||'',{type:'date'}),input('last_date','До даты',q.get('last_date')||'',{type:'date'})]),table(['Событие','Когда','Где','Организатор','Видимость'],(data.events||[]).map(x=>[button(x.title,()=>eventView(x),'quiet'),date(x.start_time,true),x.location,x.creator_full_name,status(x.is_public?'Общее':'Личное')])),pager('events',q,data.paginated_metadata)));
}
function eventView(x) {
  const can=admin()||(x.creator_id===me.id&&me.role!=='student');
  modal(x.title,el('div',{},details([['Когда',date(x.start_time,true)],['Где',x.location],['Организатор',x.creator_full_name],['Видимость',x.is_public?'Общее событие':'Личное событие']]),el('p',{class:'prose'},x.description||'Описание не добавлено.'),can?actions(button('Редактировать',()=>eventForm(x),'primary'),button('Удалить',()=>remove('Удалить событие',x.title,()=>api.deleteEvent(x.id)),'danger')):null));
}
function eventForm(x) {
  modal(x?'Редактировать событие':'Новое событие',form([input('title','Название',x?.title,{required:true,maxlength:255}),input('location','Место',x?.location,{required:true}),input('start_time','Дата и время',x?.start_time?.replace(' ','T').slice(0,16),{type:'datetime-local',required:true}),input('description','Что нужно сделать',x?.description,{type:'textarea',maxlength:5000}),input('is_public','Общее событие: видно коллегам',x?.is_public??true,{type:'checkbox'})],'Сохранить',async data=>{const payload={...nullable(data),is_public:data.is_public==='on'};if(x)await api.updateEvent(x.id,payload);else await api.createEvent(payload);closeModal();route();}));
}
async function users(q) {
  const data=await api.getUsers(),search=(q.get('search')||'').toLowerCase();
  const filtered=data.filter(u=>(!q.get('role')||u.role===q.get('role'))&&`${u.full_name} ${u.email||''}`.toLowerCase().includes(search));
  return el('div',{},head('Коллеги и доступ','Аккаунты кафедры. Только администраторы назначают роли и управляют доступом.',button('Создать аккаунт',()=>userForm(),'primary')),sheet(filterBar('users',q,[input('search','Имя или почта',q.get('search')||'',{placeholder:'Найти коллегу…'}),select('role','Роль',q.get('role')||'',[['','Все роли'],...choose(roles)])]),table(['Коллега','Роль','Кабинет','Доступ'],filtered.map(u=>[el('div',{},link(u.full_name,`#/users/${u.id}`,'record-link'),el('small',{},u.email)),roles[u.role]||u.role,u.office,status(u.is_active?'Активен':'Отключён',u.is_active?'good':'warn')])),el('div',{class:'sheet-foot'},`${filtered.length} аккаунтов`)));
}
async function profile(u) {
  const own=u.id===me.id;
  const root=el('div',{},head(own?'Мой профиль':u.full_name,`${roles[u.role]||u.role}${u.position?' · '+u.position:''}`,admin()?button('Редактировать профиль',()=>userForm(u),'primary'):null),sheet(sh(u.full_name,own?'Личная рабочая область':'Профиль сотрудника'),body(details([['Почта',u.email],['Кабинет',u.office],['Телефон',u.phone],['Дата рождения',date(u.date_of_birth)],['Роль',roles[u.role]],['Доступ',status(u.is_active?'Активен':'Отключён',u.is_active?'good':'warn')]])),body(actions(link('Публикации',`#/articles?author_id=${encodeURIComponent(u.id)}`,'btn'),own?link('Мои заметки','#/notes','btn'):null))));
  if(u.avatar)root.append(sheet(sh('Фото профиля'),body(await blobImage(api.avatarUrl(u.id),u.full_name))));
  if(admin())root.append(sheet(sh('Фото профиля'),body(upload('Загрузить аватар',file=>api.uploadUserAvatar(u.id,file)),button('Удалить аватар',()=>remove('Удалить аватар',u.full_name,()=>api.deleteUserAvatar(u.id)),'quiet'))),button(u.is_active?'Отключить доступ':'Включить доступ',()=>confirmAction(u.is_active?'Отключить доступ':'Включить доступ',`Профиль: ${u.full_name}. История операций сохранится.`,async()=>{if(u.is_active)await api.deactivateUser(u.id);else await api.activateUser(u.id);route();}),'danger'));
  const history=await api.getUserHistory(u.id);root.append(sheet(sh('История ключей'),table(['Ключ','Операция','Дата','Комментарий'],(history||[]).map(h=>[link(`Ключ ${h.key_id}`,`#/keys/${h.key_id}`),{issue:'Выдача',return:'Возврат',lost:'Утеря'}[h.action_type]||h.action_type,date(h.timestamp,true),h.comment]))));return root;
}
function userForm(u) {
  modal(u?'Профиль и права':'Новый аккаунт',form([input('full_name','Имя и фамилия',u?.full_name,{required:true,minlength:3}),input('email','Почта',u?.email,{type:'email',required:true}),...(!u?[input('password','Начальный пароль','',{type:'password',required:true,minlength:8,autocomplete:'new-password'})]:[]),select('role','Роль',u?.role||'staff',choose(roles)),el('p',{class:'form-hint'},'Администратор имеет полный доступ, включая назначение других администраторов.'),input('position','Должность',u?.position),el('div',{class:'form-grid'},input('office','Кабинет',u?.office),input('phone','Телефон',u?.phone,{type:'tel'})),input('date_of_birth','Дата рождения',u?.date_of_birth?.slice(0,10),{type:'date'})],'Сохранить',async data=>{if(u)await api.updateUser(u.id,{...nullable(data),is_active:u.is_active});else await api.createUser(nullable(data));if(u?.id===me.id)me=await api.getMe();closeModal();route();}));
}
async function notes(id) {
  const list=await request('/notes'),selected=id?list.find(n=>String(n.id)===String(id)):null;
  const editor=sheet(sh(selected?'Редактирование заметки':'Новая заметка','Видна только вам'),body(form([input('title','Заголовок',selected?.title,{required:true,maxlength:200}),input('body','Текст заметки',selected?.body,{type:'textarea',rows:15})],'Сохранить заметку',async data=>{const result=await request(selected?`/notes/${selected.id}`:'/notes',selected?'PUT':'POST',data);toast('Заметка сохранена');navTo(`notes/${selected?.id||result.id}`);})),selected?body(button('Удалить заметку',()=>remove('Удалить заметку',selected.title,()=>request(`/notes/${selected.id}`,'DELETE')),'danger')):null);
  editor.classList.add('note-editor');
  return el('div',{},head('Мои заметки','Личные записи о работе и исследованиях. Сохраняются в вашем аккаунте.',link('Новая заметка','#/notes','btn primary')),el('div',{class:'note-grid'},sheet(sh('Записи',`${list.length} заметок`),list.length?list.map(n=>el('a',{href:`#/notes/${n.id}`,class:'note-item'},el('strong',{},n.title),el('small',{},date(n.updated_at)))):body(el('p',{},'Сохраните первую заметку справа.'))),editor));
}
async function reference(q) {
  const list=await request('/reference'),search=(q.get('search')||'').toLowerCase();
  const filtered=list.filter(x=>`${x.category} ${x.name} ${x.value} ${x.notes}`.toLowerCase().includes(search));
  return el('div',{},head('Справочник','Полезные сведения кафедры. Актуальные записи поддерживают администраторы.',admin()?button('Добавить запись',()=>referenceForm(),'primary'):null),sheet(filterBar('reference',q,[input('search','Поиск по справочнику',q.get('search')||'',{placeholder:'Тема, название или значение…'})]),table(['Раздел','Название','Сведения','Примечание',...(admin()?['']:[])],filtered.map(x=>[x.category,x.name,external(x.value,x.value),x.notes,...(admin()?[actions(button('Изменить',()=>referenceForm(x),'quiet'),button('Удалить',()=>remove('Удалить запись',x.name,()=>request(`/reference/${x.id}`,'DELETE')),'quiet'))]:[])]),'Администратор может добавить контакты служб, инструкции и полезные ссылки.')));
}
function referenceForm(x) {modal(x?'Редактировать запись':'Новая запись справочника',form([input('category','Раздел',x?.category,{required:true}),input('name','Название',x?.name,{required:true}),input('value','Сведения или ссылка',x?.value,{required:true,type:'textarea'}),input('notes','Примечание',x?.notes,{type:'textarea'})],'Сохранить',async data=>{await request(x?`/reference/${x.id}`:'/reference',x?'PUT':'POST',data);closeModal();route();}));}
function assistants() {
  return el('div',{},head('Ассистенты','Инструменты научной работы и обучения.'),sheet(sh('Исследовательские инструменты','Раздел готов для подключения сервисов'),el('div',{class:'tool-row'},el('div',{},el('h3',{},'Поиск научных статей'),el('p',{},'Подбор публикаций по теме исследования с источниками и ссылками.')),status('Планируется')),el('div',{class:'tool-row'},el('div',{},el('h3',{},'Еженедельная сводка'),el('p',{},'Новые работы по вашим темам в кратком обзоре. Рассылка пока не подключена.')),status('Планируется')),el('div',{class:'tool-row'},el('div',{},el('h3',{},'Совместная рукопись'),el('p',{},'Редактор статей с комментариями и одновременной работой соавторов. Это следующий этап развития.')),status('Планируется'))),sheet(sh('Уже можно использовать'),body(actions(link('Вести личные заметки','#/notes','btn primary'),link('Работать с публикациями','#/articles','btn')))));
}
async function publicKey(id) {
  const x=await request(`/public/keys/${encodeURIComponent(id)}`);
  return el('div',{},head(`Ключ ${x.key_number}`,x.room_description||'Запрос ключа кафедры'),sheet(sh('Запросить ключ'),body(status(keyStatuses[x.status]||x.status,x.status==='available'?'good':'warn'),el('p',{class:'privacy'},'Укажите, кто вы и зачем нужен ключ. Эти сведения увидит администратор. Заявка не означает, что ключ уже выдан.'),form([input('name','Ваше имя','',{required:true,minlength:3,maxlength:200}),input('affiliation','Откуда вы: кафедра, группа или организация','',{required:true,maxlength:300}),input('purpose','Для чего нужен ключ','',{required:true,type:'textarea',maxlength:2000})],'Отправить заявку',async(data,node)=>{await request(`/public/keys/${encodeURIComponent(id)}/requests`,'POST',data);node.replaceChildren(el('h3',{},'Заявка отправлена'),el('p',{role:'status'},'Обратитесь к администратору для подтверждения и получения ключа.'));}))));
}
async function requestsPage() {
  const list=await request('/key-requests');
  return el('div',{},head('Заявки на ключи','Гостевые запросы. Перед выдачей проверьте сведения и назначьте ответственного сотрудника.'),sheet(table(['Гость','Откуда','Цель','Ключ','Статус','Действия'],list.map(x=>[x.name,x.affiliation,x.purpose,x.key_number||x.key_id,status({pending:'Ожидает решения',approved:'Подтверждена',rejected:'Отклонена'}[x.status]||x.status,x.status==='pending'?'warn':''),x.status==='pending'?actions(button('Подтвердить',()=>approveRequest(x),'primary'),button('Отклонить',()=>confirmAction('Отклонить заявку',`Заявитель: ${x.name}`,async()=>{await request(`/key-requests/${x.id}/reject`,'POST',{});route();}),'quiet')):'—']))));
}
async function approveRequest(x) {
  const people=await api.getActiveUsers();modal('Подтвердить выдачу гостю',form([el('p',{},`${x.name} · ${x.affiliation}. ${x.purpose}`),select('user_id','Ответственный сотрудник',people[0]?.id||'',people.map(p=>[p.id,p.full_name])),el('p',{class:'form-hint'},'Сотрудник будет записан ответственным держателем ключа, сведения о госте сохранятся в заявке.')],'Выдать ключ',async data=>{await request(`/key-requests/${x.id}/approve`,'POST',data);closeModal();route();}));
}
document.querySelector('.skip').addEventListener('click',event=>{event.preventDefault();content.focus();});
window.addEventListener('hashchange',route);
window.addEventListener('auth:logout',()=>{me=null;closeModal();navTo('login');});
document.addEventListener('keydown',e=>{if(e.key==='Escape')document.getElementById('rail').classList.remove('open');});
try {if(await api.refresh()) me=await api.getMe();} catch {api.clearToken();}
route();
