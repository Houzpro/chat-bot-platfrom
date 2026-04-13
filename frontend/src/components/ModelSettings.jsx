import React from 'react'
import { X, Sliders, Thermometer, Target, Hash, FileText, MessageSquare } from 'lucide-react'
import './ModelSettings.css'

function ModelSettings({ settings, onChange, onClose }) {
  const handleChange = (key, value) => {
    onChange({ ...settings, [key]: value })
  }

  const resetDefaults = async () => {
    try {
      const response = await fetch('/api/v1/config/defaults')
      if (response.ok) {
        const defaults = await response.json()
        onChange(defaults)
      } else {
        console.error('Failed to load defaults: HTTP', response.status)
      }
    } catch (error) {
      console.error('Failed to load defaults:', error)
    }
  }

  return (
    <div className="settings-overlay" onClick={onClose}>
      <div className="settings-panel" onClick={(e) => e.stopPropagation()}>
        <div className="settings-header">
          <div className="settings-title">
            <Sliders size={24} />
            <h2>Параметры модели</h2>
          </div>
          <button className="close-btn" onClick={onClose}>
            <X size={20} />
          </button>
        </div>

        <div className="settings-content">
          <div className="setting-group">
            <label>
              <Thermometer size={18} />
              Temperature
              <span className="setting-value">{settings.temperature}</span>
            </label>
            <input
              type="range"
              min="0"
              max="2"
              step="0.1"
              value={settings.temperature}
              onChange={(e) => handleChange('temperature', parseFloat(e.target.value))}
            />
            <p className="setting-hint">
              Контролирует случайность. Выше = более креативно, ниже = более предсказуемо
            </p>
          </div>

          <div className="setting-group">
            <label>
              <Target size={18} />
              Top P
              <span className="setting-value">{settings.top_p}</span>
            </label>
            <input
              type="range"
              min="0"
              max="1"
              step="0.05"
              value={settings.top_p}
              onChange={(e) => handleChange('top_p', parseFloat(e.target.value))}
            />
            <p className="setting-hint">
              Nucleus sampling. Выбирает из топ вероятных токенов с суммарной вероятностью top_p
            </p>
          </div>

          <div className="setting-group">
            <label>
              <Hash size={18} />
              Top K
              <span className="setting-value">{settings.top_k}</span>
            </label>
            <input
              type="range"
              min="1"
              max="100"
              step="1"
              value={settings.top_k}
              onChange={(e) => handleChange('top_k', parseInt(e.target.value))}
            />
            <p className="setting-hint">
              Ограничивает выбор к топ K наиболее вероятным токенам
            </p>
          </div>

          <div className="setting-group">
            <label>
              <FileText size={18} />
              Max New Tokens
              <span className="setting-value">{settings.max_new_tokens}</span>
            </label>
            <input
              type="range"
              min="32"
              max="2048"
              step="32"
              value={settings.max_new_tokens}
              onChange={(e) => handleChange('max_new_tokens', parseInt(e.target.value))}
            />
            <p className="setting-hint">
              Максимальная длина генерируемого ответа в токенах
            </p>
          </div>

          <div className="setting-group">
            <label>
              <MessageSquare size={18} />
              Контекст диалога (сообщений)
              <span className="setting-value">{settings.context_window || 0}</span>
            </label>
            <input
              type="range"
              min="0"
              max="50"
              step="2"
              value={settings.context_window || 0}
              onChange={(e) => handleChange('context_window', parseInt(e.target.value))}
            />
            <p className="setting-hint">
              Количество последних сообщений, отправляемых модели для контекста. 0 = глобальная настройка сервера
            </p>
          </div>

          <div className="setting-group">
            <label className="checkbox-label">
              <input
                type="checkbox"
                checked={settings.do_sample}
                onChange={(e) => handleChange('do_sample', e.target.checked)}
              />
              <span>Do Sample</span>
            </label>
            <p className="setting-hint">
              Включает sampling. Если выключено, используется greedy decoding (детерминированный выбор)
            </p>
          </div>

          <div className="setting-group">
            <label>
              Инструкция ассистенту
            </label>
            <textarea
              value={settings.user_prompt}
              onChange={(e) => handleChange('user_prompt', e.target.value)}
              rows="3"
              placeholder="Инструкция для ассистента..."
            />
            <p className="setting-hint">
              Инструкция для модели, определяющая её поведение и стиль ответов
            </p>
          </div>

          <div className="settings-actions">
            <button className="btn-reset" onClick={resetDefaults}>
              Сбросить на дефолт
            </button>
            <button className="btn-apply" onClick={onClose}>
              Применить
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}

export default ModelSettings
