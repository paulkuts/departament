// ===== Утилиты UI =====

const UI = {
    // Показать уведомление
    toast(message, type = 'info') {
        const container = document.getElementById('toast-container');
        const toast = document.createElement('div');
        toast.className = `toast toast-${type}`;
        toast.textContent = message;
        container.appendChild(toast);

        setTimeout(() => {
            toast.style.animation = 'slideIn 0.3s ease reverse';
            setTimeout(() => toast.remove(), 300);
        }, 3000);
    },

    // Открыть модальное окно
    openModal(title, bodyHTML) {
        document.getElementById('modal-title').textContent = title;
        document.getElementById('modal-body').innerHTML = bodyHTML;
        document.getElementById('modal-overlay').classList.add('active');
    },

    // Закрыть модальное окно
    closeModal() {
        document.getElementById('modal-overlay').classList.remove('active');
    },

    // Подтверждение действия
    confirm(message) {
        return window.confirm(message);
    },

    // Экранирование HTML
    escape(str) {
        if (str === null || str === undefined) return '';
        const div = document.createElement('div');
        div.textContent = str;
        return div.innerHTML;
    },

    // Форматирование даты
    formatDate(dateStr) {
        if (!dateStr) return '—';
        const date = new Date(dateStr);
        return date.toLocaleString('ru-RU', {
            day: '2-digit',
            month: '2-digit',
            year: 'numeric',
            hour: '2-digit',
            minute: '2-digit',
        });
    },

    // Название роли
    roleName(role) {
        const names = {
            student: 'Студент',
            teacher: 'Преподаватель',
            staff: 'Сотрудник',
        };
        return names[role] || role;
    },

    // Название статуса ключа
    statusName(status) {
        const names = {
            available: 'Свободен',
            issued: 'Выдан',
            lost: 'Утерян',
        };
        return names[status] || status;
    },

    // Название действия
    actionName(action) {
        const names = {
            issue: 'Выдача',
            return: 'Возврат',
            lost: 'Утеря',
        };
        return names[action] || action;
    },
};

// Закрытие модалки по клику на оверлей и крестик
document.getElementById('modal-overlay').addEventListener('click', (e) => {
    if (e.target.id === 'modal-overlay') {
        UI.closeModal();
    }
});

document.getElementById('modal-close').addEventListener('click', () => {
    UI.closeModal();
});

// Закрытие по Escape
document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') {
        UI.closeModal();
    }
});