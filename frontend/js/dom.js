export function el(tag, attrs = {}, ...children) {
  const node = document.createElement(tag);
  for (const [key, value] of Object.entries(attrs)) {
    if (key.startsWith('on')) node.addEventListener(key.slice(2), value);
    else if (key === 'class') node.className = value;
    else if (key === 'value') node.value = value ?? '';
    else if (value === true) node.setAttribute(key, '');
    else if (value !== false && value != null) node.setAttribute(key, String(value));
  }
  for (const child of children.flat(Infinity)) {
    if (child != null && child !== false) node.append(child instanceof Node ? child : document.createTextNode(String(child)));
  }
  return node;
}
export function toast(message) {
  const box = el('div', {class:'toast'}, message);
  document.getElementById('toasts').append(box);
  setTimeout(() => box.remove(), 6000);
}
export function button(label, fn, kind = 'secondary') {
  return el('button', {type:'button', class:`btn ${kind}`, onclick: async event => {
    const target = event.currentTarget;
    if (target.disabled) return;
    target.disabled = true;
    try { await fn(event); } catch (error) { toast(error.message); }
    finally { target.disabled = false; }
  }}, label);
}
export const link = (label, href, className = '') => el('a', {href, class:className}, label);
export function external(label, url) {
  try {
    const parsed = new URL(url);
    if (['http:', 'https:'].includes(parsed.protocol)) return el('a', {href:parsed.href, target:'_blank', rel:'noopener noreferrer'}, label);
  } catch {}
  return el('span', {}, label || '—');
}
let fieldId = 0;
export function field(name, label, value = '', options = {}) {
  const {type = 'text', choices, required = false, ...attrs} = options;
  const id = `field-${++fieldId}`;
  let input;
  if (choices) {
    input = el('select', {name, id, required, ...attrs}, choices.map(([v, text]) => el('option', {value:v}, text)));
    input.value = value ?? '';
  } else if (type === 'textarea') input = el('textarea', {name, id, rows:5, required, ...attrs}, value ?? '');
  else input = el('input', {name, id, type, required, ...attrs, value: type === 'checkbox' ? 'on' : value});
  if (type === 'checkbox') { input.checked = !!value; return el('label', {class:'check', for:id}, input, label); }
  return el('div', {class:'field'}, el('label', {for:id}, label, required ? el('span', {class:'required'}, ' *') : null), input);
}
export function form(fields, submitLabel, save) {
  const error = el('p', {class:'form-error', role:'alert'});
  const submit = el('button', {type:'submit', class:'btn primary'}, submitLabel);
  const node = el('form', {class:'form', onsubmit: async event => {
    event.preventDefault();
    if (submit.disabled) return;
    error.textContent = '';
    submit.disabled = true;
    submit.textContent = 'Сохраняем…';
    try { await save(Object.fromEntries(new FormData(node)), node); }
    catch (err) { error.textContent = err.message; error.scrollIntoView({block:'nearest'}); }
    finally { submit.disabled = false; submit.textContent = submitLabel; }
  }}, fields, error, el('div', {class:'form-actions'}, submit));
  return node;
}
export function modal(title, content) {
  const dialog = document.getElementById('dialog');
  const previous = document.activeElement;
  dialog.replaceChildren(el('header', {class:'dialog-header'}, el('h2', {id:'dialog-title'}, title), button('Закрыть', () => dialog.close(), 'quiet')), content);
  if (!dialog.open) dialog.showModal();
  dialog.addEventListener('close', () => { if (previous?.isConnected) previous.focus(); }, {once:true});
  return dialog;
}
export function confirmAction(title, description, action) {
  modal(title, el('div', {}, el('p', {class:'prose'}, description), el('div', {class:'form-actions'},
    button('Отмена', closeModal), button(title, async () => { await action(); closeModal(); }, 'danger'))));
}
export function closeModal() { document.getElementById('dialog').close(); }
// Время в базе и в API — UTC. Беззоновую строку («2026-10-05 13:07:25» — так отдаёт
// API и движок конвертера) браузер иначе прочитал бы как местное время и показал
// UTC под видом московского; поэтому зону подставляем сами. Показываем и вводим
// время по Москве: кафедра в Белгороде, все журналы идут по МСК.
const MSK = 'Europe/Moscow';
const parseStamp = value => {
  if (!value) return null;
  const text = String(value).trim();
  const naive = /^\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}(:\d{2})?(\.\d+)?$/.test(text);
  const d = new Date(naive ? text.replace(' ', 'T') + 'Z' : text);
  return Number.isNaN(d.getTime()) ? null : d;
};
export const date = (value, time = false) => {
  const d = parseStamp(value);
  return d ? d.toLocaleString('ru-RU', time ? {dateStyle:'medium', timeStyle:'short', timeZone:MSK} : {dateStyle:'medium', timeZone:MSK}) : '—';
};
// Значение для поля datetime-local: человек вводит московское время, а в API оно
// уходит как есть (сервер разбирает его по МСК и хранит в UTC).
export const dateLocal = value => {
  const d = parseStamp(value);
  if (!d) return '';
  const parts = new Intl.DateTimeFormat('ru-RU', {timeZone:MSK, year:'numeric', month:'2-digit', day:'2-digit', hour:'2-digit', minute:'2-digit', hour12:false}).formatToParts(d);
  const get = type => (parts.find(p => p.type === type) || {}).value;
  return `${get('year')}-${get('month')}-${get('day')}T${get('hour')}:${get('minute')}`;
};
export const status = (text, kind = '') => el('span', {class:`status ${kind}`}, text);
// Описания строк → узлы таблицы. Отдельная функция: реестр дописывает строки
// кнопкой «Загрузить все» без перерисовки страницы.
export function buildRows(rows, columnCount) {
  return rows.flatMap(row => {
    const isObj = Array.isArray(row);
    const cells = isObj ? row : (row.cells || []);
    const line = el('tr', isObj ? {} : {class: row.cls || null}, cells.map(cell => el('td', {}, cell ?? '—')));
    if (isObj) return [line];
    if (row.under) return [line, el('tr', {class:'row-under'}, el('td', {colspan:String(columnCount)}, row.under))];
    if (row.expand) {
      // Строка-группа: под ней мини-таблица единиц. Точка слева только обозначает
      // группу: раскрывает и сворачивает клик по любой части строки.
      const body = el('tr', {class:'group-body' + (row.alt ? ' alt' : '')}, el('td', {colspan:String(columnCount)}, row.expand));
      body.hidden = !row.open;
      const dot = line.querySelector('.group-toggle');
      const toggle = () => {
        body.hidden = !body.hidden;
        const open = !body.hidden;
        line.classList.toggle('expanded', open);
        line.setAttribute('aria-expanded', String(open));
        dot?.setAttribute('aria-expanded', String(open));
        row.onToggle?.(open);
      };
      line.tabIndex = 0;
      line.setAttribute('role', 'button');
      line.setAttribute('aria-expanded', String(row.open));
      if (row.label) line.setAttribute('aria-label', `Группа «${row.label}»`);
      if (row.open) line.classList.add('expanded');
      line.addEventListener('click', toggle);
      line.addEventListener('keydown', event => {
        if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); toggle(); }
      });
      return [line, body];
    }
    if (row.onClick || row.onDblClick) {
      line.classList.add('clickable');
      // Одиночный клик — мини-карточка, двойной — страница. Одиночный ждёт 220 мс.
      let clickTimer = null;
      line.addEventListener('click', event => {
        if (event.target.closest('a,button')) return;
        if (!row.onDblClick) { row.onClick?.(event); return; }
        clearTimeout(clickTimer);
        clickTimer = setTimeout(() => { clickTimer = null; row.onClick?.(event); }, 220);
      });
      line.addEventListener('dblclick', event => {
        if (event.target.closest('a,button')) return;
        clearTimeout(clickTimer);
        clickTimer = null;
        row.onDblClick(event);
      });
    }
    return [line];
  });
}

export function table(headers, rows, emptyText = 'Пока нет записей. Они появятся здесь после добавления.', extraClass = '', opts = {}) {
  if (!rows.length) return el('div', {class:'empty'}, el('h3', {}, 'Здесь пока пусто'), el('p', {}, emptyText));
  const lines = buildRows(rows, headers.length);
  // Заголовок — строка или {label, href, dir}: во втором случае это сортировка,
  // dir («asc»/«desc») показывает текущий порядок стрелкой.
  const cellHead = h => {
    if (typeof h === 'string') return el('th', {scope:'col'}, h);
    const arrow = h.dir === 'asc' ? '▲' : h.dir === 'desc' ? '▼' : '';
    return el('th', {scope:'col', 'aria-sort':h.dir === 'asc' ? 'ascending' : h.dir === 'desc' ? 'descending' : 'none'},
      el('a', {class:'sort-head' + (h.dir ? ' active' : ''), href:h.href, title:h.title || 'Сортировать по колонке'},
        h.label, el('span', {class:'sort-arrow', 'aria-hidden':'true'}, arrow)));
  };
  const head = el('thead', {}, el('tr', {}, headers.map(cellHead)));
  // Двойной клик по пустому месту шапки — сброс сортировки (по ссылкам не срабатываем).
  if (opts.headerDblClick) {
    head.addEventListener('dblclick', e => {
      if (e.target.closest('a')) return;
      opts.headerDblClick();
    });
    if (opts.headerTitle) head.querySelectorAll('th').forEach(th => { th.title = opts.headerTitle; });
  }
  return el('div', {class:'table-scroll' + (extraClass ? ' ' + extraClass : ''), tabindex:'0', role:'region', 'aria-label':'Реестр. На узком экране прокручивается по горизонтали'},
    el('table', {}, head, el('tbody', {}, lines)));
}
export const actions = (...nodes) => el('div', {class:'actions'}, nodes);
export const sheet = (...nodes) => el('section', {class:'sheet'}, nodes);
export function details(pairs) {
  return el('dl', {class:'details'}, pairs.map(([label, value]) => el('div', {}, el('dt', {}, label), el('dd', {}, value ?? '—'))));
}
