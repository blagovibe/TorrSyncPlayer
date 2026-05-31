package container

import (
	"fmt"
	"sync"
)

// Lifecycle тип жизненного цикла сервиса
type Lifecycle int

const (
	// Singleton один экземпляр на весь контейнер
	Singleton Lifecycle = iota
	// Transient новый экземпляр при каждом запросе
	Transient
)

// serviceRegistration регистрация сервиса в контейнере
type serviceRegistration struct {
	factory   func() interface{}
	lifecycle Lifecycle
	instance  interface{}
	once      sync.Once
}

// Container простой DI контейнер
type Container struct {
	mu       sync.RWMutex
	services map[string]*serviceRegistration
}

// New создает новый DI контейнер
func New() *Container {
	return &Container{
		services: make(map[string]*serviceRegistration),
	}
}

// RegisterSingleton регистрирует сервис как singleton
func (c *Container) RegisterSingleton(name string, factory func() interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.services[name] = &serviceRegistration{
		factory:   factory,
		lifecycle: Singleton,
	}
}

// RegisterTransient регистрирует сервис как transient
func (c *Container) RegisterTransient(name string, factory func() interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.services[name] = &serviceRegistration{
		factory:   factory,
		lifecycle: Transient,
	}
}

// Resolve получает сервис по имени
func (c *Container) Resolve(name string) (interface{}, error) {
	c.mu.RLock()
	reg, exists := c.services[name]
	c.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("service not registered: %s", name)
	}

	if reg.lifecycle == Singleton {
		reg.once.Do(func() {
			reg.instance = reg.factory()
		})
		return reg.instance, nil
	}

	// Transient - создаем новый экземпляр
	return reg.factory(), nil
}

// ResolveOrPanic получает сервис по имени, паникует если не найден
func (c *Container) ResolveOrPanic(name string) interface{} {
	service, err := c.Resolve(name)
	if err != nil {
		panic(err)
	}
	return service
}

// Has проверяет, зарегистрирован ли сервис
func (c *Container) Has(name string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	_, exists := c.services[name]
	return exists
}

// Clear очищает все регистрации
func (c *Container) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.services = make(map[string]*serviceRegistration)
}
