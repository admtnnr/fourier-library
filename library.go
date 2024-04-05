// Package library provides a simple library system that allows adding books, creating
// accounts, checking out books, and returning books. The library system is
// thread-safe and can be used concurrently. The library system can be exported
// to and imported from JSON.
package library

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"sync"
)

var (
	// ErrBookNotExist is returned when a book does not exist.
	ErrBookNotExist = errors.New("book does not exist")
	// ErrAccountNotExist is returned when an account does not exist.
	ErrAccountNotExist = errors.New("account does not exist")
	// ErrCheckoutNotExist is returned when a checkout does not exist.
	ErrCheckoutNotExist = errors.New("checkout does not exist")
)

// Library represents a simple library system.
type Library struct {
	mu sync.RWMutex

	books    map[int]*Book
	accounts map[int]*Account

	// Create an index for fast lookup of checkouts by account. The primary
	// use cases for this are:
	// 1. enforce max account checkout limit
	// 2. print account summary
	//
	// Performance rationale: the number of accounts could be large so
	// we get value out of the O(1) lookup by account, but the number of
	// checkouts is explicitly limited to 4 per account so a linear scan of
	// the checkouts for an account is not a performance concern and could
	// even be faster than doing a nested map due to the constant factors.
	checkoutsByAccount map[int][]*Checkout

	// Create an index for fast lookup of checkouts by book. The primary
	// use cases for this are:
	// 1. prevent checkout of book with no available copies
	// 2. prevent removal of copies that exceeds available copies
	//
	// Performance rationale: the number of books could be large so we get
	// value out of the O(1) lookup by book. The number of checkouts isn't
	// explicitly limited like the accounts case, but libraries tend to not
	// keep more than a dozen copies of the even the most popular books due
	// to space constraints and limiting checkout duration forcing
	// turnover, so a linear scan of the checkouts for a book is not a
	// performance concern and, again, could even be faster than doing a
	// nested map due to the constant factors.
	checkoutsByBook map[int][]*Checkout
}

// Account represents a library account.
type Account struct {
	ID   int    // Unique identifier for the account.
	Name string // Name of the account holder, not required to be unique.
}

// Book represents a book in the library catalog.
type Book struct {
	ID    int    // Unique identifier for the book.
	Name  string // Name of the book, not required to be unique.
	Count int    // Number of copies of the book available in the library.
}

// Checkout represents a book checkout by an account.
type Checkout struct {
	BookID    int // ID of the book being checked out.
	AccountID int // ID of the account checking out the book.
}

// New creates a new library system.
func New() *Library {
	return &Library{
		books:              make(map[int]*Book),
		accounts:           make(map[int]*Account),
		checkoutsByAccount: make(map[int][]*Checkout),
		checkoutsByBook:    make(map[int][]*Checkout),
	}
}

// AddBook adds a book to the library catalog.
//
// If a book with the provided ID already exists, an error is returned. The
// count must be non-negative.
func (l *Library) AddBook(id int, name string, count int) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if _, ok := l.books[id]; ok {
		return fmt.Errorf("book already exists")
	}

	if count < 0 {
		return fmt.Errorf("cannot add negative copies")
	}

	l.books[id] = &Book{
		ID:    id,
		Name:  name,
		Count: count,
	}

	return nil
}

// AddCopies adds copies of a existing book in the library catalog.
//
// If a book with the provided ID does not exist, an error is returned. The
// count must be non-negative.
func (l *Library) AddCopies(id, count int) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	book, ok := l.books[id]
	if !ok {
		return ErrBookNotExist
	}

	if count < 0 {
		return fmt.Errorf("cannot add negative copies")
	}

	book.Count += count

	return nil
}

// RemoveCopies removes copies of a existing book in the library catalog.
//
// If a book with the provided ID does not exist, an error is returned. The
// count must be non-negative, and cannot exceed the number of available
// copies at the time of removal.
func (l *Library) RemoveCopies(id, count int) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	book, ok := l.books[id]
	if !ok {
		return ErrBookNotExist
	}

	if count < 0 {
		return fmt.Errorf("cannot remove negative copies")
	}

	if book.Count < count {
		return fmt.Errorf("cannot remove more copies than exist")
	}

	available := book.Count - len(l.checkoutsByBook[book.ID])
	if available < count {
		return fmt.Errorf("cannot remove more copies of %s (%d) than are available to check out (%d)", book.Name, book.ID, available)
	}

	book.Count -= count

	return nil
}

// CreateAccount creates a new account in the library system.
//
// If an account with the provided ID already exists, an error is returned.
func (l *Library) CreateAccount(id int, name string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if _, ok := l.accounts[id]; ok {
		return fmt.Errorf("account already exists")
	}

	l.accounts[id] = &Account{
		ID:   id,
		Name: name,
	}

	return nil
}

// CheckoutBook checks out a book to an account.
//
// If the account or book does not exist, an error is returned.
// If the account already has 4 books checked out currently, an error is returned.
// If the account already has a copy of the book checked out currently, an
// error is returned.
func (l *Library) CheckoutBook(accountID, bookID int) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	account, ok := l.accounts[accountID]
	if !ok {
		return ErrAccountNotExist
	}

	book, ok := l.books[bookID]
	if !ok {
		return ErrBookNotExist
	}

	checkouts := l.checkoutsByAccount[account.ID]

	if len(checkouts) >= 4 {
		return fmt.Errorf("%s (%d) cannot checkout more than 4 books at a time", account.Name, account.ID)
	}

	for _, checkout := range checkouts {
		if checkout.AccountID == account.ID && checkout.BookID == book.ID {
			return fmt.Errorf("%s (%d) cannot checkout more than one copy of %s (%d)", account.Name, account.ID, book.Name, book.ID)
		}
	}

	checkout := &Checkout{
		AccountID: account.ID,
		BookID:    book.ID,
	}

	l.checkoutsByAccount[account.ID] = append(l.checkoutsByAccount[account.ID], checkout)
	l.checkoutsByBook[book.ID] = append(l.checkoutsByBook[book.ID], checkout)

	return nil
}

// ReturnBook returns a book to the library.
//
// If the account or book does not exist, an error is returned. If the book is
// not checked out by the account, an error is returned.
func (l *Library) ReturnBook(accountID, bookID int) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	account, ok := l.accounts[accountID]
	if !ok {
		return ErrAccountNotExist
	}

	book, ok := l.books[bookID]
	if !ok {
		return ErrBookNotExist
	}

	matchCheckout := func(checkout *Checkout) bool {
		return checkout.AccountID == account.ID && checkout.BookID == book.ID
	}

	if !slices.ContainsFunc(l.checkoutsByAccount[account.ID], matchCheckout) {
		return ErrCheckoutNotExist
	}

	l.checkoutsByAccount[account.ID] = slices.DeleteFunc(l.checkoutsByAccount[account.ID], matchCheckout)
	l.checkoutsByBook[book.ID] = slices.DeleteFunc(l.checkoutsByBook[book.ID], matchCheckout)

	return nil
}

// Account returns an account by ID.
func (l *Library) Account(id int) *Account {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.accounts[id]
}

// EachBook calls the provided function for each book in the library.
//
// The function exists to allow thread-safe iteration of the books in the
// library.
func (l *Library) EachBook(fn func(book *Book)) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	for _, book := range l.books {
		fn(book)
	}
}

// EachAccount calls the provided function for each account in the library.
//
// The function exists to allow thread-safe iteration of the accounts in the
// library.
func (l *Library) EachAccount(fn func(account *Account)) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	for _, account := range l.accounts {
		fn(account)
	}
}

// Book returns a book by ID.
func (l *Library) Book(id int) *Book {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.books[id]
}

// CheckoutsByAccount returns the checkouts for an account by ID.
func (l *Library) CheckoutsByAccount(id int) []*Checkout {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.checkoutsByAccount[id]
}

// CheckoutsByBook returns the checkouts for a book by ID.
func (l *Library) CheckoutsByBook(id int) []*Checkout {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.checkoutsByBook[id]
}

// Export writes the library state to a writer in JSON format.
//
// Export uses the same format as Import to allow for round-trip serialization
// and persistence across invocations.
func (l *Library) Export(w io.Writer) error {
	l.mu.RLock()
	defer l.mu.RUnlock()

	enc := json.NewEncoder(w)

	for _, book := range l.books {
		inv := Invocation{
			Command: &AddBook{
				ID:    book.ID,
				Name:  book.Name,
				Count: book.Count,
			},
		}

		if err := enc.Encode(&inv); err != nil {
			return fmt.Errorf("failed to write library state, %w", err)
		}
	}

	for _, account := range l.accounts {
		inv := Invocation{
			Command: &CreateAccount{
				ID:   account.ID,
				Name: account.Name,
			},
		}

		if err := enc.Encode(&inv); err != nil {
			return fmt.Errorf("failed to write library state, %w", err)
		}
	}

	for _, checkouts := range l.checkoutsByAccount {
		for _, checkout := range checkouts {
			inv := Invocation{
				Command: &CheckoutBook{
					AccountID: checkout.AccountID,
					BookID:    checkout.BookID,
				},
			}

			if err := enc.Encode(&inv); err != nil {
				return fmt.Errorf("failed to write library state, %w", err)
			}
		}
	}

	return nil
}

// ImportOptions provides options for importing library state.
type ImportOptions struct {
	// LogOutput indicates whether to log the output of each invocation to stdout.
	//
	// This is used to avoid logging output when loading initial library
	// state, but allow for logging output when executing the user
	// commands.
	LogOutput bool
}

// Import reads the library state from a reader in JSON format.
func (l *Library) Import(r io.Reader, opts ImportOptions) error {
	dec := json.NewDecoder(r)

	for {
		var inv Invocation

		if err := dec.Decode(&inv); errors.Is(err, io.EOF) {
			return nil
		} else if err != nil {
			return fmt.Errorf("failed to read library state, %w", err)
		}

		err := inv.Exec(l)

		if opts.LogOutput {
			fmt.Fprintf(os.Stdout, "%s\n", inv.Output)
		}

		if err != nil {
			return err
		}
	}
}
