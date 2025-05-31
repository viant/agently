package agently

import (
    "context"
    "fmt"
)

// ListCmd prints conversation IDs known to the history store.
type ListCmd struct{}

func (c *ListCmd) Execute(_ []string) error {
    ids, err := executorSingleton().Conversation().List(context.Background())
    if err != nil {
        return err
    }
    for _, id := range ids {
        fmt.Println(id)
    }
    return nil
}
