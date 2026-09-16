const API_BASE = '/api/v1';

class ApiClient {
    constructor(baseURL) {
        this.baseURL = baseURL;
        this.accessToken = null;
        localStorage.removeItem('access_token');
        this._refreshing = null;
    }

    setToken(token) { 
        this.accessToken = token;
    }
    
    clearToken() {
        this.accessToken = null;
        localStorage.removeItem('access_token');
    }
    
    isLoggedIn() { return !!this.accessToken; }

    parseToken() {
        if (!this.accessToken) return null;
        try {
            const payload = JSON.parse(
                atob(this.accessToken.split('.')[1].replace(/-/g, '+').replace(/_/g, '/'))
            );
            if (typeof payload.sub === 'object' && payload.sub !== null) {
                return { userId: payload.sub.UserID, role: payload.sub.Role };
            }
            return { userId: payload.sub, role: payload.role };
        } catch {
            return null;
        }
    }

    async request(endpoint, options = {}, _isRetry = false) {
        const url = `${this.baseURL}${endpoint}`;
        const headers = { 'Content-Type': 'application/json', ...(options.headers || {}) };

        if (this.accessToken) {
            headers['Authorization'] = `Bearer ${this.accessToken}`;
        }

        try {
            const response = await fetch(url, {
                credentials: 'include',
                ...options,
                headers,
            });

            if (response.status === 401 && !_isRetry && !endpoint.startsWith('/auth/') && !endpoint.startsWith('/public/')) {
                const refreshed = await this.refresh();
                if (refreshed) {
                    return this.request(endpoint, options, true);
                }
                this.clearToken();
                window.dispatchEvent(new Event('auth:logout'));
                throw new Error('Сессия истекла');
            }

            const data = await response.json().catch(() => ({}));

            if (!response.ok) {
                throw new Error(data.error || `Ошибка ${response.status}`);
            }
            return data;
        } catch (error) {
            if (error.name === 'TypeError') {
                throw new Error('Не удалось подключиться к серверу');
            }
            throw error;
        }
    }

    async refresh() {
        if (this._refreshing) return this._refreshing;

        this._refreshing = (async () => {
            try {
                const res = await fetch(`${this.baseURL}/auth/refresh`, {
                    method: 'POST',
                    credentials: 'include',
                    headers: { 'Content-Type': 'application/json' },
                });
                if (!res.ok) return false;
                const data = await res.json();
                if (data.access_token) {
                    this.setToken(data.access_token);
                    return true;
                }
                return false;
            } catch {
                return false;
            } finally {
                this._refreshing = null;
            }
        })();

        return this._refreshing;
    }

    // ─── Auth ───
    async signIn(email, password) {
        const data = await this.request('/auth/signin', {
            method: 'POST',
            body: JSON.stringify({ email, password }),
        });
        this.setToken(data.access_token);
        return this.parseToken();
    }

    async logout() {
        try { await this.request('/auth/logout', { method: 'POST' }); } catch { /* ignore */ }
        this.clearToken();
    }

    // ─── Profile ───
    getMe() { return this.request('/me'); }
    avatarUrl(userId) { return `${this.baseURL}/avatars/${userId}`; }

    // ─── Users ───
    getUsers()           { return this.request('/users'); }
    getActiveUsers(role = null) {
        const q = new URLSearchParams();
        if (role) q.append('role', role);
        const qs = q.toString();
        return this.request(`/users/active${qs ? '?' + qs : ''}`);
    }
    getUser(id)          { return this.request(`/users/${id}`); }
    createUser(data)     { return this.request('/users', { method: 'POST', body: JSON.stringify(data) }); }
    updateUser(id, data) { return this.request(`/users/${id}`, { method: 'PUT', body: JSON.stringify(data) }); }
    deactivateUser(id)   { return this.request(`/users/${id}`, { method: 'DELETE' }); }
    activateUser(id)     { return this.request(`/users/${id}/activate`, { method: 'POST' }); }
    getUserHistory(id)   { return this.request(`/users/${id}/history`); }
    deleteUserAvatar(userId) { return this.request(`/users/${userId}/avatar`, { method: 'DELETE' }); }

    async uploadUserAvatar(userId, file, _isRetry = false) {
        const formData = new FormData();
        formData.append('avatar', file);
        const headers = {};
        if (this.accessToken) headers['Authorization'] = `Bearer ${this.accessToken}`;

        const res = await fetch(`${this.baseURL}/users/${userId}/avatar`, {
            method: 'POST', credentials: 'include', headers, body: formData,
        });
        if (res.status === 401 && !_isRetry) {
            const refreshed = await this.refresh();
            if (refreshed) return this.uploadUserAvatar(userId, file, true);
            this.clearToken();
            window.dispatchEvent(new Event('auth:logout'));
            throw new Error('Сессия истекла');
        }
        const data = await res.json().catch(() => ({}));
        if (!res.ok) throw new Error(data.error || `Ошибка ${res.status}`);
        return data;
    }

    // ─── Keys ───
    getKeys(status = '') { return this.request(`/keys${status ? '?status=' + status : ''}`); }
    getKey(id)           { return this.request(`/keys/${id}`); }
    createKey(data)      { return this.request('/keys', { method: 'POST', body: JSON.stringify(data) }); }
    updateKey(id, data)  { return this.request(`/keys/${id}`, { method: 'PUT', body: JSON.stringify(data) }); }
    issueKey(id, data)   { return this.request(`/keys/${id}/issue`, { method: 'POST', body: JSON.stringify(data) }); }
    returnKey(id, data)  { return this.request(`/keys/${id}/return`, { method: 'POST', body: JSON.stringify(data) }); }
    markLost(id, data)   { return this.request(`/keys/${id}/lost`, { method: 'POST', body: JSON.stringify(data) }); }
    restoreKey(id, data) { return this.request(`/keys/${id}/restore`, { method: 'POST', body: JSON.stringify(data) }); }
    getKeyHistory(id)    { return this.request(`/keys/${id}/history`); }
    getKeyHolder(id)     { return this.request(`/keys/${id}/holder`); }

    // ─── Inventory ───
    getInventory(params = {}) {
        const q = new URLSearchParams();
        if (params.limit)     q.append('limit', params.limit);
        if (params.offset)    q.append('offset', params.offset);
        if (params.search)    q.append('search', params.search);
        if (params.inventory) q.append('inventory', params.inventory);
        if (params.status)    q.append('status', params.status);
        if (params.type)      q.append('type', params.type);
        const qs = q.toString();
        return this.request(`/inventory${qs ? '?' + qs : ''}`);
    }
    getInventoryById(id)         { return this.request(`/inventory/${id}`); }
    getInventoryNumbers(search = '') { return this.request(`/inventory-numbers${search ? '?search=' + encodeURIComponent(search) : ''}`); }
    createInventory(data)        { return this.request('/inventory', { method: 'POST', body: JSON.stringify(data) }); }
    updateInventory(id, data)    { return this.request(`/inventory/${id}`, { method: 'PUT', body: JSON.stringify(data) }); }
    deleteInventory(id)          { return this.request(`/inventory/${id}`, { method: 'DELETE' }); }
    getExpiredVerification(l, o) { return this.request(`/inventory/expired-verification?limit=${l}&offset=${o}`); }

    // ─── Photos ───
    getPhotos(inventoryId) { return this.request(`/inventory/${inventoryId}/photos`); }
    deletePhoto(photoId)   { return this.request(`/photos/${photoId}`, { method: 'DELETE' }); }
    photoUrl(photoId)      { return `${this.baseURL}/photos/${photoId}`; }
    qrCodeUrl(inventoryId) { return `${this.baseURL}/inventory/${inventoryId}/qr`; }

    async uploadPhoto(inventoryId, file, _isRetry = false) {
        const formData = new FormData();
        formData.append('photo', file);

        const headers = {};
        if (this.accessToken) headers['Authorization'] = `Bearer ${this.accessToken}`;

        const res = await fetch(`${this.baseURL}/inventory/${inventoryId}/photos`, {
            method: 'POST',
            credentials: 'include',
            headers,
            body: formData,
        });

        if (res.status === 401 && !_isRetry) {
            const refreshed = await this.refresh();
            if (refreshed) return this.uploadPhoto(inventoryId, file, true);
            this.clearToken();
            window.dispatchEvent(new Event('auth:logout'));
            throw new Error('Сессия истекла');
        }

        const data = await res.json().catch(() => ({}));
        if (!res.ok) throw new Error(data.error || `Ошибка ${res.status}`);
        return data;
    }

    // ─── Articles ───
    getArticles(params = {}) {
        const q = new URLSearchParams();
        if (params.limit)     q.append('limit', params.limit);
        if (params.offset)    q.append('offset', params.offset);
        if (params.search)    q.append('search', params.search);
        if (params.status)    q.append('status', params.status);
        if (params.author_id) q.append('author_id', params.author_id);
        const qs = q.toString();
        return this.request(`/articles${qs ? '?' + qs : ''}`);
    }
    getArticle(id)          { return this.request(`/articles/${id}`); }
    createArticle(data)     { return this.request('/articles', { method: 'POST', body: JSON.stringify(data) }); }
    updateArticle(id, data) { return this.request(`/articles/${id}`, { method: 'PUT', body: JSON.stringify(data) }); }
    deleteArticle(id)       { return this.request(`/articles/${id}`, { method: 'DELETE' }); }

    // ─── Events ───
    getEvents(params = {}) {
        const q = new URLSearchParams();
        if (params.limit)      q.append('limit', params.limit);
        if (params.offset)     q.append('offset', params.offset);
        if (params.title)      q.append('title', params.title);
        if (params.first_date) q.append('first_date', params.first_date);
        if (params.last_date)  q.append('last_date', params.last_date);
        if (params.is_public !== undefined && params.is_public !== null) {
            q.append('is_public', String(params.is_public));
        }
        if (params.all)        q.append('all', params.all);
        const qs = q.toString();
        return this.request(`/events${qs ? '?' + qs : ''}`);
    }
    getEvent(id)          { return this.request(`/events/${id}`); }
    createEvent(data)     { return this.request('/events', { method: 'POST', body: JSON.stringify(data) }); }
    updateEvent(id, data) { return this.request(`/events/${id}`, { method: 'PUT', body: JSON.stringify(data) }); }
    deleteEvent(id)       { return this.request(`/events/${id}`, { method: 'DELETE' }); }
}

export const api = new ApiClient(API_BASE);
