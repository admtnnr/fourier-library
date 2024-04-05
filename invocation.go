package library

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Invocation represents an action to be executed against the Library and the
// human readable output of the executiong of that action.
//
// The rationale for introducing a concept like Invocation separate from the
// Library is to allow Invocation to manage the primary concerns around
// interacting with user inputs and ouputs for the CLI while keeping the Library
// focused on the core behavior of catalog and account management.
//
// Should we want to introduce a new interface for interacting with the Library
// other than command files, we can do so without changing the Library.
type Invocation struct {
	// RawCommand is the raw command that was parsed from the input JSON.
	//
	// This is used to determine the concrete Command type to unmarshal the
	// Arguments into in Command.
	RawCommand Command
	// Command is concrete Command type that was derived from the RawCommand.
	//
	// Concrete Command types currently supported are:
	// - *AddBook
	// - *AddCopies
	// - *RemoveCopies
	// - *CreateAccount
	// - *CheckoutBook
	// - *ReturnBook
	// - *PrintCatalog
	// - *PrintAccounts
	Command any
	// Output is the human readable output of the execution of the Command.
	Output string
}

// Command represents an action to be executed against the Library and the
// arguments required for that action.
//
// Commands are parsed incrementally to allow for deserializing the Arguments
// directly into the correct Command type based on the command name.
type Command struct {
	// Name is the name of the command. Currently, the following commands are supported:
	//
	// - ADD_BOOK
	// - ADD_COPIES
	// - REMOVE_COPIES
	// - CREATE_ACCOUNT
	// - CHECKOUT_BOOK
	// - RETURN_BOOK
	// - PRINT_CATALOG
	// - PRINT_ACCOUNTS
	Name string `json:"name"`
	// Arguments are the serialized arguments for the command. The
	// arguments are deserialized separately into the correct Command type
	// in Invocation.Command based on the Name.
	Arguments json.RawMessage `json:"arguments"`
}

// Exec executes the Command against the Library and sets the human readable
// output for optional display to the user.
//
// The majority of the code in this method is concerned with setting the most
// useful human readable output, particularly around error conditions.
func (inv *Invocation) Exec(l *Library) error {
	switch cmd := inv.Command.(type) {
	case *AddBook:
		err := l.AddBook(cmd.ID, cmd.Name, cmd.Count)
		if err != nil {
			inv.Output = fmt.Sprintf("%s (%d) could not be added to the catalog, %v", cmd.Name, cmd.ID, err)
			return err
		}

		inv.Output = fmt.Sprintf("%s (%d) with %d copies added to the catalog", cmd.Name, cmd.ID, cmd.Count)
	case *AddCopies:
		err := l.AddCopies(cmd.ID, cmd.Count)
		if errors.Is(err, ErrBookNotExist) {
			inv.Output = fmt.Sprintf("could not add %d copies, book (%d) does not exist", cmd.ID)
			return err
		}

		book := l.Book(cmd.ID)

		if err != nil {
			inv.Output = fmt.Sprintf("%s (%d) could not add %d copies, %v", book.Name, book.ID, cmd.Count, err)
			return err
		}

		inv.Output = fmt.Sprintf("%s (%d) added %d copies", book.Name, book.ID, cmd.Count)
	case *RemoveCopies:
		err := l.RemoveCopies(cmd.ID, cmd.Count)
		if errors.Is(err, ErrBookNotExist) {
			inv.Output = fmt.Sprintf("could not remove %d copies, book (%d) does not exist", cmd.ID)
			return err
		}

		book := l.Book(cmd.ID)

		if err != nil {
			inv.Output = fmt.Sprintf("%s (%d) could not remove %d copies, %v", book.Name, book.ID, cmd.Count, err)
			return err
		}

		inv.Output = fmt.Sprintf("%s (%d) removed %d copies", book.Name, book.ID, cmd.Count)
	case *CreateAccount:
		inv.Command = CreateAccount{}
		err := l.CreateAccount(cmd.ID, cmd.Name)
		if err != nil {
			inv.Output = fmt.Sprintf("%s (%d) could not create account, %v", cmd.Name, cmd.ID, err)
			return err
		}

		inv.Output = fmt.Sprintf("%s (%d) created account", cmd.Name, cmd.ID)
	case *CheckoutBook:
		err := l.CheckoutBook(cmd.AccountID, cmd.BookID)
		if errors.Is(err, ErrAccountNotExist) {
			inv.Output = fmt.Sprintf("could not checkout book, account (%d) does not exist", cmd.AccountID)
			return err
		}

		account := l.Account(cmd.AccountID)

		if errors.Is(err, ErrBookNotExist) {
			inv.Output = fmt.Sprintf("%s (%d) could not checkout book, book (%d) does not exist", account.Name, account.ID, cmd.BookID)
			return err
		}

		book := l.Book(cmd.BookID)

		if err != nil {
			inv.Output = fmt.Sprintf("%s (%d) could not checkout %s (%d), %v", account.Name, account.ID, book.Name, book.ID, err)
			return err
		}

		inv.Output = fmt.Sprintf("%s (%d) checked out %s (%d)", account.Name, account.ID, book.Name, book.ID)
	case *ReturnBook:
		err := l.ReturnBook(cmd.AccountID, cmd.BookID)
		if errors.Is(err, ErrAccountNotExist) {
			inv.Output = fmt.Sprintf("could not return book, account (%d) does not exist", cmd.AccountID)
			return err
		}

		account := l.Account(cmd.AccountID)

		if errors.Is(err, ErrBookNotExist) {
			inv.Output = fmt.Sprintf("%s (%d) could not return book, book (%d) does not exist", account.Name, account.ID, cmd.BookID)
			return err
		}

		book := l.Book(cmd.BookID)

		if errors.Is(err, ErrCheckoutNotExist) {
			inv.Output = fmt.Sprintf("%s (%d) could not return %s (%d), no checkout exists", account.Name, account.ID, book.Name, book.ID)
			return err
		}

		if err != nil {
			inv.Output = fmt.Sprintf("%s (%d) could not return %s (%d)", account.Name, account.ID, book.Name, book.ID, err)
			return err
		}

		inv.Output = fmt.Sprintf("%s (%d) returned %s (%d)", account.Name, account.ID, book.Name, book.ID)
	case *PrintCatalog:
		var sb strings.Builder

		sb.WriteString("# Library Catalog\n")

		l.EachBook(func(book *Book) {
			fmt.Fprintf(&sb, "## %s (%d)\n", book.Name, book.ID)
			fmt.Fprintf(&sb, "Copies: %d\n", book.Count)

			checkouts := l.CheckoutsByBook(book.ID)

			fmt.Fprintf(&sb, "Checked Out: %d\n", len(checkouts))

			sb.WriteRune('\n')
		})

		inv.Output = sb.String()
	case *PrintAccounts:
		var sb strings.Builder

		sb.WriteString("# Accounts\n\n")

		l.EachAccount(func(account *Account) {
			fmt.Fprintf(&sb, "## %s (%d)\n", account.Name, account.ID)

			sb.WriteString("Checked Out Books:\n")

			checkouts := l.CheckoutsByAccount(account.ID)

			for _, checkout := range checkouts {
				book := l.Book(checkout.BookID)

				fmt.Fprintf(&sb, "- %s (%d)\n", book.Name, book.ID)
			}

			sb.WriteRune('\n')
		})

		inv.Output = sb.String()
	default:
		return fmt.Errorf("exec: unknown command type, %T", inv.Command)
	}

	return nil
}

// MarshalJSON marshals the Invocation into JSON.
//
// For example, an invocation of an AddBook command like the following:
//
//	Invocation{
//	  Command: &AddBook{
//	    ID: 1,
//	    Name: "The Great Gatsby",
//	    Count: 5,
//	  },
//	}
//
// Would be marshaled as follows:
//
//	{
//	  "name": "ADD_BOOK",
//	  "arguments": {
//	    "id": 1,
//	    "name": "The Great Gatsby",
//	    "count": 5
//	  }
//	}
func (inv *Invocation) MarshalJSON() ([]byte, error) {
	var cmd Command

	switch inv.Command.(type) {
	case *AddBook:
		cmd.Name = "ADD_BOOK"
	case *AddCopies:
		cmd.Name = "ADD_COPIES"
	case *RemoveCopies:
		cmd.Name = "REMOVE_COPIES"
	case *CreateAccount:
		cmd.Name = "CREATE_ACCOUNT"
	case *CheckoutBook:
		cmd.Name = "CHECKOUT_BOOK"
	case *ReturnBook:
		cmd.Name = "RETURN_BOOK"
	case *PrintCatalog:
		cmd.Name = "PRINT_CATALOG"
	case *PrintAccounts:
		cmd.Name = "PRINT_ACCOUNTS"
	default:
		return nil, fmt.Errorf("marshal: unknown command type, %T", inv.Command)
	}

	inv.RawCommand = cmd

	bs, err := json.Marshal(inv.Command)
	if err != nil {
		return nil, err
	}

	inv.RawCommand.Arguments = bs

	return json.Marshal(inv.RawCommand)
}

// UnmarshalJSON unmarshals the Invocation from JSON into the Command.
//
// For example, an invocation of an serialized AddBook command like the following:
//
//	{
//	  "name": "ADD_BOOK",
//	  "arguments": {
//	    "id": 1,
//	    "name": "The Great Gatsby",
//	    "count": 5
//	  }
//	}
//
// Would be unmarshaled into the Invocation as follows:
//
//	Invocation{
//	 Command: &AddBook{
//	   ID: 1,
//	   Name: "The Great Gatsby",
//	   Count: 5,
//	 },
//	}
func (inv *Invocation) UnmarshalJSON(bs []byte) error {
	if err := json.Unmarshal(bs, &inv.RawCommand); err != nil {
		return err
	}

	rbs := []byte(inv.RawCommand.Arguments)

	// GOTCHA: The `Command` types *MUST* be pointer types to a concrete type
	// to enable the `json.Decoder` logic to use reflection to determine the
	// concrete type of the command to know which struct fields to
	// unmarshal the arguments into.
	//
	// `Invocation.Command` is an `interface{}` (`any`) type so taking a
	// pointer to `inv.Command` results in the `json.Decoder` receiving a
	// `*interface{}` causing it to unmarshal incorrectly.
	switch inv.RawCommand.Name {
	case "ADD_BOOK":
		inv.Command = &AddBook{}
	case "ADD_COPIES":
		inv.Command = &AddCopies{}
	case "REMOVE_COPIES":
		inv.Command = &RemoveCopies{}
	case "CREATE_ACCOUNT":
		inv.Command = &CreateAccount{}
	case "CHECKOUT_BOOK":
		inv.Command = &CheckoutBook{}
	case "RETURN_BOOK":
		inv.Command = &ReturnBook{}
	case "PRINT_CATALOG":
		inv.Command = &PrintCatalog{}
		return nil
	case "PRINT_ACCOUNTS":
		inv.Command = &PrintAccounts{}
		return nil
	default:
		return fmt.Errorf("unmarshal: unknown command type, %s", inv.RawCommand.Name)
	}

	return json.Unmarshal(rbs, inv.Command)
}

// AddBook represents the arguments for the ADD_BOOK command.
type AddBook struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// AddCopies represents the arguments for the ADD_COPIES command.
type AddCopies struct {
	ID    int `json:"id"`
	Count int `json:"count"`
}

// RemoveCopies represents the arguments for the REMOVE_COPIES command.
type RemoveCopies struct {
	ID    int `json:"id"`
	Count int `json:"count"`
}

// CreateAccount represents the arguments for the CREATE_ACCOUNT command.
type CreateAccount struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// CheckoutBook represents the arguments for the CHECKOUT_BOOK command.
type CheckoutBook struct {
	AccountID int `json:"accountId"`
	BookID    int `json:"bookId"`
}

// ReturnBook represents the arguments for the RETURN_BOOK command.
type ReturnBook struct {
	AccountID int `json:"accountId"`
	BookID    int `json:"bookId"`
}

// PrintCatalog represents the arguments for the PRINT_CATALOG command.
//
// PrintCatlog has no arguments, but the type is required to implement the
// implicit Command interface required by the Invocation.
type PrintCatalog struct{}

// PrintAccounts represents the arguments for the PRINT_ACCOUNTS command.
//
// PrintAccounts has no arguments, but the type is required to implement the
// implicit Command interface required by the Invocation.
type PrintAccounts struct{}
